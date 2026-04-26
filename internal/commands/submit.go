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
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Submit(ctx context.Context, start string, message string) error {
	if message == "" {
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

	files, err := workspace.Scan(a.Root, a.Config)
	if err != nil {
		return err
	}

	blobFiles := workspace.FilterByMode(files, rules.ModeBlob)
	gitFiles := workspace.FilterByMode(files, rules.ModeGit)
	d := manifest.Compare(m, blobFiles)

	changedBlobFiles := append([]workspace.FileInfo{}, d.Added...)
	changedBlobFiles = append(changedBlobFiles, d.Modified...)

	for _, f := range changedBlobFiles {
		exists, err := a.Store.Has(ctx, f.SHA256)
		if err != nil {
			return err
		}

		if !exists {
			in, err := os.Open(f.AbsPath)
			if err != nil {
				return err
			}

			if err := a.Store.Put(ctx, f.SHA256, in); err != nil {
				_ = in.Close()
				return err
			}

			_ = in.Close()
			fmt.Printf("uploaded %s %s\n", shortSHA(f.SHA256), f.Path)
		}

		m.Entries[f.Path] = manifest.Entry{
			Path:      f.Path,
			SHA256:    f.SHA256,
			Size:      f.Size,
			MTimeUnix: f.MTimeUnix,
			Kind:      f.Kind,
			Deleted:   false,
		}
	}

	for _, e := range d.Deleted {
		e.Deleted = true
		m.Entries[e.Path] = e
		fmt.Printf("marked deleted %s\n", e.Path)
	}

	blobChanged := len(changedBlobFiles) > 0 || len(d.Deleted) > 0
	if blobChanged {
		if err := manifest.Save(a.ManifestPath, m); err != nil {
			return err
		}
	}

	g := gdgit.New(a.Root)

	pathsToAdd := []string{
		".gamedepot/config.yaml",
		".gitignore",
		a.Config.ManifestPath,
		"Config",
		"Source",
		"Docs",
		"External/WebLinks",
		"External/Tech",
		"External/Launchers",
	}
	pathsToAdd = append(pathsToAdd, workspace.Paths(gitFiles)...)

	if err := g.AddA(uniqueNonEmpty(pathsToAdd)...); err != nil {
		return err
	}

	status, err := g.StatusPorcelain()
	if err != nil {
		return err
	}

	if strings.TrimSpace(status) == "" {
		fmt.Println("No GameDepot changes")
		return nil
	}

	if err := g.Commit(message); err != nil {
		return err
	}

	fmt.Printf(
		"Submitted: %d blob added, %d blob modified, %d blob deleted, %d git-managed matched\n",
		len(d.Added),
		len(d.Modified),
		len(d.Deleted),
		len(gitFiles),
	)

	return nil
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
