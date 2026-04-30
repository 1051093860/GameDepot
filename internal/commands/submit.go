package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/submitplan"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type SubmitOptions struct {
	AllowUnmanaged bool
	DryRun         bool
}

func Submit(ctx context.Context, start string, message string) error {
	return SubmitWithOptions(ctx, start, message, SubmitOptions{})
}

func SubmitWithOptions(ctx context.Context, start string, message string, opts SubmitOptions) error {
	if message == "" && !opts.DryRun {
		return fmt.Errorf("commit message is required")
	}

	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			m = manifest.New(a.Config.ProjectID)
		} else {
			return err
		}
	}
	m.Normalize()
	prunedManifestEntries := pruneManifestToManagedPaths(&m)

	plan, err := submitplan.Build(ctx, a, m, opts.AllowUnmanaged)
	if err != nil {
		return err
	}
	if len(plan.Blockers) > 0 {
		return fmt.Errorf("submit is blocked:\n%s", formatSubmitBlockers(plan.Blockers, 20))
	}
	if opts.DryRun {
		for _, it := range plan.Items {
			fmt.Printf("%-14s %-7s -> %-7s %s\n", it.Action, it.PreviousStorage, it.DesiredStorage, it.Path)
		}
		return nil
	}

	g := gdgit.New(a.Root)
	counts := map[submitplan.Action]int{}

	for _, item := range plan.Items {
		counts[item.Action]++
		switch item.Action {
		case submitplan.ActionUploadBlob, submitplan.ActionGitToBlob, submitplan.ActionUpdateBlob:
			if !item.LocalExists {
				return fmt.Errorf("cannot upload missing local file: %s", item.Path)
			}
			if err := submitplan.UploadBlob(ctx, a, item.File); err != nil {
				return err
			}
			m.Upsert(manifest.Entry{Path: item.Path, Storage: manifest.StorageBlob, SHA256: item.LocalSHA256, Size: item.Size, MTimeUnix: item.File.MTimeUnix, Kind: item.Kind})
			tracked, err := g.IsTracked(item.Path)
			if err != nil {
				return err
			}
			if tracked {
				if err := g.RmCached(item.Path); err != nil {
					return err
				}
			}
		case submitplan.ActionBlobToGit, submitplan.ActionGitAdd:
			if !item.LocalExists {
				return fmt.Errorf("cannot add missing local file to Git: %s", item.Path)
			}
			m.Upsert(manifest.Entry{Path: item.Path, Storage: manifest.StorageGit, Size: item.Size, MTimeUnix: item.File.MTimeUnix, Kind: item.Kind})
			if err := g.AddForce(item.Path); err != nil {
				return err
			}
		case submitplan.ActionIgnore:
			m.Remove(item.Path)
			tracked, err := g.IsTracked(item.Path)
			if err != nil {
				return err
			}
			if tracked {
				if err := g.RmCached(item.Path); err != nil {
					return err
				}
			}
		case submitplan.ActionRemove:
			old, ok := m.Get(item.Path)
			if ok {
				old.Deleted = true
				m.Upsert(old)
				if old.Storage == manifest.StorageGit {
					_ = g.AddUpdate(item.Path)
				}
			}
		case submitplan.ActionNoop:
			// Keep existing manifest entry. For blob files this means local SHA matches current route,
			// but we still repair a stale Git index route if someone previously git-added the file.
			if item.PreviousStorage == manifest.StorageBlob {
				tracked, err := g.IsTracked(item.Path)
				if err != nil {
					return err
				}
				if tracked {
					if err := g.RmCached(item.Path); err != nil {
						return err
					}
				}
			}
		case submitplan.ActionReview:
			if !opts.AllowUnmanaged {
				return fmt.Errorf("review file reached executor unexpectedly: %s", item.Path)
			}
		}
	}

	if prunedManifestEntries > 0 {
		m.Touch()
	}
	if err := manifest.Save(a.ManifestPath, m); err != nil {
		return err
	}

	// Strategy B: GameDepot routes Content/** through the manifest/blob store, then
	// lets Git stage every other non-ignored project file directly. The UE5
	// .gitignore prevents generated folders and blob-routed Content binaries from
	// being added back to Git.
	if err := g.AddA("."); err != nil {
		return err
	}

	staged, err := g.StagedNames()
	if err != nil {
		return err
	}

	if strings.TrimSpace(staged) == "" {
		fmt.Println("No GameDepot changes")
		return nil
	}

	if err := g.Commit(message); err != nil {
		return err
	}

	fmt.Printf("Submitted: git_add=%d upload_blob=%d git_to_blob=%d blob_to_git=%d update_blob=%d remove=%d ignore=%d\n",
		counts[submitplan.ActionGitAdd], counts[submitplan.ActionUploadBlob], counts[submitplan.ActionGitToBlob], counts[submitplan.ActionBlobToGit], counts[submitplan.ActionUpdateBlob], counts[submitplan.ActionRemove], counts[submitplan.ActionIgnore])

	return nil
}

func pruneManifestToManagedPaths(m *manifest.Manifest) int {
	m.Normalize()
	removed := 0
	for p := range m.Entries {
		if !workspace.IsGameDepotManagedPath(p) {
			delete(m.Entries, p)
			removed++
		}
	}
	return removed
}

func gitAddExistingForce(g gdgit.Git, root string, paths ...string) error {
	for _, p := range uniqueNonEmpty(paths) {
		abs, err := workspace.SafeJoin(root, p)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err == nil {
			if err := g.AddForce(p); err != nil {
				return err
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func formatSubmitBlockers(blockers []submitplan.Blocker, limit int) string {
	if limit <= 0 || limit > len(blockers) {
		limit = len(blockers)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		fmt.Fprintf(&b, "  %s  %s\n", blockers[i].Path, blockers[i].Reason)
	}
	if len(blockers) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(blockers)-limit)
	}
	b.WriteString("Set a GameDepot rule or use --allow-unmanaged for advanced/manual workflows.\n")
	return b.String()
}

func formatReviewFiles(files []workspace.FileInfo, limit int) string {
	if limit <= 0 || limit > len(files) {
		limit = len(files)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		f := files[i]
		fmt.Fprintf(&b, "  %s  %d bytes  %s\n", f.Path, f.Size, f.Kind)
	}
	if len(files) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(files)-limit)
	}
	b.WriteString("Add a GameDepot rule, move the file into a known directory, or use --allow-unmanaged.\n")
	return b.String()
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

var _ = rules.ModeGit
