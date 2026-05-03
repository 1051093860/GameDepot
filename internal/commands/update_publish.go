package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/localindex"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/submitplan"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type ProgressEvent struct {
	Phase   string
	Path    string
	Message string
	Current int
	Total   int
}

type ProgressFunc func(ProgressEvent)

type UpdateOptions struct {
	Force    bool
	Strict   bool
	Progress ProgressFunc
}

type PublishOptions struct {
	DryRun   bool
	Progress ProgressFunc
}

type updateAction struct {
	Kind      string
	Path      string
	LocalOID  string
	BaseOID   string
	RemoteOID string
}

type publishAction struct {
	Kind      string
	Path      string
	LocalOID  string
	BaseOID   string
	RemoteOID string
	File      workspace.FileInfo
}

func emitProgress(fn ProgressFunc, phase, path, message string, current, total int) {
	if fn != nil {
		fn(ProgressEvent{Phase: phase, Path: path, Message: message, Current: current, Total: total})
	}
}

func Update(ctx context.Context, start string, opts UpdateOptions) error {
	if opts.Force && opts.Strict {
		return fmt.Errorf("update --force and --strict cannot be used together")
	}
	emitProgress(opts.Progress, "load", "", "Loading GameDepot project", 0, 0)
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	if opts.Strict {
		emitProgress(opts.Progress, "strict", "", "Checking local Content changes", 0, 0)
		changes, err := localContentChanges(a)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			return fmt.Errorf("update --strict aborted: local Content changes exist:\n%s", formatUpdateConflicts(changes, 20))
		}
	}
	g := gdgit.New(a.Root)
	if g.HasAnyRemote() {
		emitProgress(opts.Progress, "fetch", "", "Pulling latest refs", 0, 0)
		if _, err := g.Run("pull", "--ff-only"); err != nil {
			return err
		}
	} else {
		fmt.Println("No git remote configured; updating from local refs only.")
	}
	return materializeRefs(ctx, a, opts)
}

func materializeRefs(ctx context.Context, a *app.App, opts UpdateOptions) error {
	refStore := gdrefs.NewStore(a.Root)
	remoteRefs, err := refStore.LoadAll()
	if err != nil {
		return err
	}
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return err
	}
	localFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return err
	}
	localByPath := map[string]workspace.FileInfo{}
	for _, f := range localFiles {
		localByPath[f.Path] = f
	}

	actions := []updateAction{}
	conflicts := []updateAction{}
	seen := map[string]bool{}

	for _, p := range gdrefs.SortedPaths(remoteRefs) {
		seen[p] = true
		r := remoteRefs[p]
		remoteOID := gdrefs.EnsureOID(r.OID)
		local, hasLocal := localByPath[p]
		baseOID := idx.BaseOID(p)
		if !hasLocal {
			switch {
			case opts.Force:
				actions = append(actions, updateAction{Kind: "download", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
			case baseOID == "":
				actions = append(actions, updateAction{Kind: "download", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
			case remoteOID == baseOID:
				actions = append(actions, updateAction{Kind: "keep-local-deleted", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
			default:
				conflicts = append(conflicts, updateAction{Kind: "conflict-local-deleted-remote-modified", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
			}
			continue
		}
		localOID := gdrefs.EnsureOID(local.SHA256)
		switch {
		case localOID == remoteOID:
			actions = append(actions, updateAction{Kind: "unchanged", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		case opts.Force:
			actions = append(actions, updateAction{Kind: "download", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		case baseOID == "":
			conflicts = append(conflicts, updateAction{Kind: "conflict-unindexed", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		case localOID == baseOID && remoteOID != baseOID:
			actions = append(actions, updateAction{Kind: "download", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		case localOID != baseOID && remoteOID == baseOID:
			actions = append(actions, updateAction{Kind: "keep-local", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		default:
			conflicts = append(conflicts, updateAction{Kind: "conflict", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID})
		}
	}

	// Refs removed by Git mean remote deletion. Remove only clean local files.
	for _, p := range sortedFileInfoPaths(localByPath) {
		if seen[p] {
			continue
		}
		baseOID := idx.BaseOID(p)
		localOID := gdrefs.EnsureOID(localByPath[p].SHA256)
		if opts.Force {
			actions = append(actions, updateAction{Kind: "remove", Path: p, LocalOID: localOID, BaseOID: baseOID})
			continue
		}
		if baseOID == "" {
			actions = append(actions, updateAction{Kind: "local-untracked", Path: p, LocalOID: localOID})
			continue
		}
		if localOID == baseOID {
			actions = append(actions, updateAction{Kind: "remove", Path: p, LocalOID: localOID, BaseOID: baseOID})
		} else {
			conflicts = append(conflicts, updateAction{Kind: "conflict-deleted-remote", Path: p, LocalOID: localOID, BaseOID: baseOID})
		}
	}

	// If both local and remote have deleted a file, there is no local file and
	// no remote ref left to drive an action above. The local base entry still
	// needs to be forgotten, otherwise future strict updates keep reporting a
	// phantom local deletion.
	for _, p := range idx.Paths() {
		if seen[p] {
			continue
		}
		if _, hasLocal := localByPath[p]; hasLocal {
			continue
		}
		actions = append(actions, updateAction{Kind: "forget-deleted", Path: p, BaseOID: idx.BaseOID(p)})
	}

	if len(conflicts) > 0 {
		if err := saveUpdateConflicts(a.Root, conflicts); err != nil {
			return err
		}
		return fmt.Errorf("update blocked by local/remote Content conflicts:\n%s\nrun `gamedepot conflicts` and resolve with `gamedepot resolve <path> --remote` or `gamedepot resolve <path> --local`", formatUpdateConflicts(conflicts, 20))
	}

	downloaded, removed, kept, unchanged := 0, 0, 0, 0
	for i, act := range actions {
		emitProgress(opts.Progress, act.Kind, act.Path, fmt.Sprintf("%s %s", act.Kind, act.Path), i+1, len(actions))
		switch act.Kind {
		case "download":
			r := remoteRefs[act.Path]
			dst, err := workspace.SafeJoin(a.Root, act.Path)
			if err != nil {
				return err
			}
			sha := gdrefs.SHAFromOID(r.OID)
			if err := downloadBlobToFile(ctx, a.Store, sha, dst); err != nil {
				return err
			}
			actual, err := blob.SHA256File(dst)
			if err != nil {
				return err
			}
			if actual != sha {
				return fmt.Errorf("sha256 mismatch after download: %s", act.Path)
			}
			idx.SetBase(act.Path, gdrefs.EnsureOID(r.OID))
			downloaded++
			fmt.Printf("updated %s %s\n", shortSHA(sha), act.Path)
		case "remove":
			dst, err := workspace.SafeJoin(a.Root, act.Path)
			if err != nil {
				return err
			}
			if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
				return err
			}
			idx.Delete(act.Path)
			removed++
			fmt.Printf("removed %s\n", act.Path)
		case "forget-deleted":
			idx.Delete(act.Path)
			removed++
			fmt.Printf("removed %s\n", act.Path)
		case "unchanged":
			idx.SetBase(act.Path, act.RemoteOID)
			unchanged++
		case "keep-local", "keep-local-deleted", "local-untracked":
			kept++
		}
	}
	if err := localindex.Save(a.Root, idx); err != nil {
		return err
	}
	_ = clearConflicts(a.Root)
	fmt.Printf("Update complete: %d updated, %d removed, %d kept local, %d unchanged\n", downloaded, removed, kept, unchanged)
	return nil
}

func Publish(ctx context.Context, start string, message string, opts PublishOptions) error {
	emitProgress(opts.Progress, "load", "", "Loading GameDepot project", 0, 0)
	if strings.TrimSpace(message) == "" && !opts.DryRun {
		return fmt.Errorf("commit message is required")
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	if st, err := loadConflictState(a.Root); err != nil {
		return err
	} else if st.Active && len(st.Conflicts) > 0 {
		return fmt.Errorf("publish blocked by active GameDepot conflicts; run `gamedepot conflicts` and resolve them first")
	}
	if shouldPreUpdateBeforePublish(a.Root) {
		emitProgress(opts.Progress, "pre-update", "", "Checking remote changes before publish", 0, 0)
		if err := Update(ctx, a.Root, UpdateOptions{Progress: opts.Progress}); err != nil {
			return fmt.Errorf("publish pre-update failed: %w", err)
		}
		// Update may have fast-forwarded the Git worktree and refreshed local-index,
		// so reload App state before computing the publish plan.
		a, err = app.Load(ctx, start)
		if err != nil {
			return err
		}
	}
	refStore := gdrefs.NewStore(a.Root)
	remoteRefs, err := refStore.LoadAll()
	if err != nil {
		return err
	}
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return err
	}
	localFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return err
	}
	localByPath := map[string]workspace.FileInfo{}
	for _, f := range localFiles {
		localByPath[f.Path] = f
	}

	actions := []publishAction{}
	blockers := []publishAction{}
	seen := map[string]bool{}
	for _, p := range sortedFileInfoPaths(localByPath) {
		seen[p] = true
		f := localByPath[p]
		localOID := gdrefs.EnsureOID(f.SHA256)
		baseOID := idx.BaseOID(p)
		ref, hasRef := remoteRefs[p]
		remoteOID := ""
		if hasRef {
			remoteOID = gdrefs.EnsureOID(ref.OID)
		}
		switch {
		case !hasRef:
			actions = append(actions, publishAction{Kind: "upload", Path: p, LocalOID: localOID, File: f})
		case localOID == remoteOID:
			actions = append(actions, publishAction{Kind: "unchanged", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID, File: f})
		case baseOID == remoteOID:
			actions = append(actions, publishAction{Kind: "upload", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID, File: f})
		case baseOID != "" && localOID == baseOID && remoteOID != baseOID:
			blockers = append(blockers, publishAction{Kind: "stale-local", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID, File: f})
		default:
			blockers = append(blockers, publishAction{Kind: "conflict", Path: p, LocalOID: localOID, BaseOID: baseOID, RemoteOID: remoteOID, File: f})
		}
	}
	for _, p := range gdrefs.SortedPaths(remoteRefs) {
		if seen[p] {
			continue
		}
		remoteOID := gdrefs.EnsureOID(remoteRefs[p].OID)
		baseOID := idx.BaseOID(p)
		if baseOID == remoteOID {
			actions = append(actions, publishAction{Kind: "delete", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
		} else {
			blockers = append(blockers, publishAction{Kind: "delete-conflict", Path: p, BaseOID: baseOID, RemoteOID: remoteOID})
		}
	}

	// Clean up stale base entries for files that no longer exist locally and no
	// longer have refs. This can happen after both local and remote deleted the
	// same asset and update has already materialized the deletion.
	for _, p := range idx.Paths() {
		if _, hasLocal := localByPath[p]; hasLocal {
			continue
		}
		if _, hasRemote := remoteRefs[p]; hasRemote {
			continue
		}
		idx.Delete(p)
	}

	if len(blockers) > 0 {
		return fmt.Errorf("publish blocked by stale/conflicting Content files:\n%s", formatPublishBlockers(blockers, 20))
	}
	if opts.DryRun {
		for _, a := range actions {
			fmt.Printf("%-10s %s\n", a.Kind, a.Path)
		}
		return nil
	}

	g := gdgit.New(a.Root)
	uploaded, deleted, unchanged := 0, 0, 0
	for i, act := range actions {
		emitProgress(opts.Progress, act.Kind, act.Path, fmt.Sprintf("%s %s", act.Kind, act.Path), i+1, len(actions))
		switch act.Kind {
		case "upload":
			if err := submitplan.UploadBlob(ctx, a, act.File); err != nil {
				return err
			}
			ref := gdrefs.NewAssetRef(act.Path, act.File.SHA256, act.File.Size, act.File.Kind)
			if err := refStore.Save(ref); err != nil {
				return err
			}
			tracked, err := g.IsTracked(act.Path)
			if err != nil {
				return err
			}
			if tracked {
				if err := g.RmCached(act.Path); err != nil {
					return err
				}
			}
			idx.SetBase(act.Path, ref.OID)
			uploaded++
		case "delete":
			if err := refStore.Delete(act.Path); err != nil {
				return err
			}
			idx.Delete(act.Path)
			deleted++
		case "unchanged":
			idx.SetBase(act.Path, act.RemoteOID)
			unchanged++
		}
	}

	emitProgress(opts.Progress, "git-add", "", "Staging GameDepot refs", len(actions), len(actions))
	if err := g.AddA("."); err != nil {
		return err
	}
	staged, err := g.StagedNames()
	if err != nil {
		return err
	}
	if strings.TrimSpace(staged) == "" {
		if err := localindex.Save(a.Root, idx); err != nil {
			return err
		}
		fmt.Println("No GameDepot changes")
		return nil
	}
	emitProgress(opts.Progress, "commit", "", "Creating Git commit", len(actions), len(actions))
	if err := g.Commit(message); err != nil {
		return err
	}
	emitProgress(opts.Progress, "push", "", "Pushing to remote", len(actions), len(actions))
	if err := Push(ctx, start); err != nil {
		return err
	}
	if err := localindex.Save(a.Root, idx); err != nil {
		return err
	}
	fmt.Printf("Published: uploaded=%d deleted=%d unchanged=%d\n", uploaded, deleted, unchanged)
	return nil
}

func shouldPreUpdateBeforePublish(root string) bool {
	g := gdgit.New(root)
	if !g.HasAnyRemote() {
		return false
	}
	branch, remote := currentBranchAndRemote(g)
	out, err := g.Run("ls-remote", "--heads", remote, branch)
	return err == nil && strings.TrimSpace(out) != ""
}

func currentBranchAndRemote(g gdgit.Git) (string, string) {
	branch, err := g.CurrentBranch()
	if err != nil || strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == "HEAD" {
		branch = "main"
	}
	remote := "origin"
	if out, err := g.Run("config", "branch."+branch+".remote"); err == nil && strings.TrimSpace(out) != "" {
		remote = strings.TrimSpace(out)
	}
	return branch, remote
}

func localContentChanges(a *app.App) ([]updateAction, error) {
	idx, err := localindex.Load(a.Root)
	if err != nil {
		return nil, err
	}
	localFiles, err := workspace.ScanManaged(a.Root, a.Config)
	if err != nil {
		return nil, err
	}
	localByPath := map[string]workspace.FileInfo{}
	for _, f := range localFiles {
		localByPath[f.Path] = f
	}
	changes := []updateAction{}
	seen := map[string]bool{}
	for _, p := range sortedFileInfoPaths(localByPath) {
		seen[p] = true
		localOID := gdrefs.EnsureOID(localByPath[p].SHA256)
		baseOID := idx.BaseOID(p)
		if baseOID == "" {
			changes = append(changes, updateAction{Kind: "local-untracked", Path: p, LocalOID: localOID})
			continue
		}
		if localOID != baseOID {
			changes = append(changes, updateAction{Kind: "local-modified", Path: p, LocalOID: localOID, BaseOID: baseOID})
		}
	}
	for _, p := range idx.Paths() {
		if seen[p] {
			continue
		}
		changes = append(changes, updateAction{Kind: "local-deleted", Path: p, BaseOID: idx.BaseOID(p)})
	}
	return changes, nil
}

func sortedFileInfoPaths(m map[string]workspace.FileInfo) []string {
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func formatUpdateConflicts(items []updateAction, limit int) string {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		it := items[i]
		fmt.Fprintf(&b, "  %s  %s  base=%s local=%s remote=%s\n", it.Kind, it.Path, shortOID(it.BaseOID), shortOID(it.LocalOID), shortOID(it.RemoteOID))
	}
	if len(items) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(items)-limit)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatPublishBlockers(items []publishAction, limit int) string {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		it := items[i]
		fmt.Fprintf(&b, "  %s  %s  base=%s local=%s remote=%s\n", it.Kind, it.Path, shortOID(it.BaseOID), shortOID(it.LocalOID), shortOID(it.RemoteOID))
	}
	if len(items) > limit {
		fmt.Fprintf(&b, "  ... and %d more\n", len(items)-limit)
	}
	return strings.TrimRight(b.String(), "\n")
}

func shortOID(oid string) string {
	sha := gdrefs.SHAFromOID(oid)
	if sha == "" {
		return "-"
	}
	return shortSHA(sha)
}
