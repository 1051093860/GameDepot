package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Sync(ctx context.Context, start string, force bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}

	restored := 0
	removed := 0
	skipped := 0

	for _, e := range m.Entries {
		if _, err := workspace.CleanRelPath(e.Path); err != nil {
			return fmt.Errorf("unsafe manifest entry %q: %w", e.Path, err)
		}

		dst, err := workspace.SafeJoin(a.Root, e.Path)
		if err != nil {
			return err
		}

		if e.Deleted {
			if err := protectLocalChangeBeforeOverwrite(ctx, a.Store, dst, e.Path, force); err != nil {
				return err
			}
			if err := os.Remove(dst); err == nil {
				removed++
				fmt.Printf("removed %s\n", e.Path)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}

		needDownload := false

		if _, err := os.Stat(dst); os.IsNotExist(err) {
			needDownload = true
		} else if err != nil {
			return err
		} else {
			localSHA, err := blob.SHA256File(dst)
			if err != nil {
				return err
			}

			if localSHA != e.SHA256 {
				if err := protectKnownOrForce(ctx, a.Store, dst, e.Path, localSHA, force); err != nil {
					return err
				}
				needDownload = true
			}
		}

		if !needDownload {
			skipped++
			continue
		}

		if err := downloadBlobToFile(ctx, a.Store, e.SHA256, dst); err != nil {
			return err
		}

		actual, err := blob.SHA256File(dst)
		if err != nil {
			return err
		}

		if actual != e.SHA256 {
			return fmt.Errorf("sha256 mismatch after download: %s", e.Path)
		}

		restored++

		fmt.Printf("restored %s %s\n", shortSHA(e.SHA256), e.Path)
	}

	fmt.Printf("Sync complete: %d restored, %d removed, %d unchanged\n", restored, removed, skipped)

	return nil
}

type getter interface {
	Get(ctx context.Context, sha256 string) (io.ReadCloser, error)
}

type knownBlobStore interface {
	getter
	Has(ctx context.Context, sha256 string) (bool, error)
}

func protectLocalChangeBeforeOverwrite(ctx context.Context, s knownBlobStore, dst string, rel string, force bool) error {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	sha, err := blob.SHA256File(dst)
	if err != nil {
		return err
	}
	return protectKnownOrForce(ctx, s, dst, rel, sha, force)
}

func protectKnownOrForce(ctx context.Context, s knownBlobStore, _ string, rel string, localSHA string, force bool) error {
	if force {
		return nil
	}
	known, err := s.Has(ctx, localSHA)
	if err != nil {
		return err
	}
	if known {
		return nil
	}
	return fmt.Errorf("refusing to overwrite local unsubmitted change: %s (%s); run `gamedepot sync --force` to discard it", rel, shortSHA(localSHA))
}

func downloadBlobToFile(ctx context.Context, s getter, sha string, dst string) error {
	r, err := s.Get(ctx, sha)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
