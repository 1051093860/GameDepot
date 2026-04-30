package restoreops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func RestoreVersion(ctx context.Context, a *app.App, relPath string, commit string, force bool) error {
	relPath, err := workspace.CleanRelPath(relPath)
	if err != nil {
		return err
	}
	if commit == "" {
		return fmt.Errorf("commit is required")
	}
	g := gdgit.New(a.Root)
	raw, err := g.ShowFileBytes(commit, a.Config.ManifestPath)
	if err != nil {
		// legacy Git-only version without manifest
		data, gitErr := g.ShowFileBytes(commit, relPath)
		if gitErr != nil {
			return err
		}
		return writeWorkingFile(a.Root, relPath, data, force)
	}
	m, err := manifest.LoadBytes(raw)
	if err != nil {
		return err
	}
	e, ok := m.Get(relPath)
	if !ok || e.Deleted {
		return fmt.Errorf("%s is not present in commit %s", relPath, commit)
	}
	if e.Storage == manifest.StorageBlob {
		return restoreBlob(ctx, a, relPath, e.SHA256, force)
	}
	data, err := g.ShowFileBytes(commit, relPath)
	if err != nil {
		return err
	}
	return writeWorkingFile(a.Root, relPath, data, force)
}

func RevertAssets(ctx context.Context, a *app.App, paths []string, force bool) error {
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}
	g := gdgit.New(a.Root)
	for _, rel := range paths {
		rel, err = workspace.CleanRelPath(rel)
		if err != nil {
			return err
		}
		e, ok := m.Get(rel)
		if !ok || e.Deleted {
			if err := trashLocalFile(a.Root, rel); err != nil {
				return err
			}
			continue
		}
		switch e.Storage {
		case manifest.StorageGit:
			if err := g.CheckoutFile(rel); err != nil {
				return err
			}
		case manifest.StorageBlob:
			if err := restoreBlob(ctx, a, rel, e.SHA256, force); err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot revert %s with storage=%s", rel, e.Storage)
		}
	}
	return nil
}

func restoreBlob(ctx context.Context, a *app.App, rel, sha string, force bool) error {
	if sha == "" {
		return fmt.Errorf("missing sha256 for %s", rel)
	}
	dst, err := workspace.SafeJoin(a.Root, rel)
	if err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(dst); err == nil {
			local, err := blob.SHA256File(dst)
			if err != nil {
				return err
			}
			if local != sha {
				if ok, _ := a.Store.Has(ctx, local); !ok {
					return fmt.Errorf("refusing to overwrite local-only change for %s; use force to discard it", rel)
				}
			}
		}
	}
	r, err := a.Store.Get(ctx, sha)
	if err != nil {
		return err
	}
	defer r.Close()
	return writeStreamFile(dst, r)
}

func writeWorkingFile(root, rel string, data []byte, force bool) error {
	dst, err := workspace.SafeJoin(root, rel)
	if err != nil {
		return err
	}
	if !force {
		if _, err := os.Stat(dst); err == nil {
			// Caller is explicitly restoring a version; allowing overwrite is normal.
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func writeStreamFile(dst string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".gamedepot-tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func trashLocalFile(root, rel string) error {
	src, err := workspace.SafeJoin(root, rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	dst := filepath.Join(root, ".gamedepot", "trash", time.Now().UTC().Format("20060102T150405"), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}
