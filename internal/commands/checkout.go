package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type CheckoutOptions struct {
	Force bool
}

// Checkout switches the Git version first, then restores blob-managed files from
// the checked-out manifest. Before switching, it removes current blob-managed
// local files that are known to be restorable so Git can replace the same path
// when the target version stores that file in Git.
func Checkout(ctx context.Context, start string, ref string, opts CheckoutOptions) error {
	if ref == "" {
		return fmt.Errorf("checkout requires a git ref")
	}

	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	if err := removeCurrentKnownBlobFiles(ctx, a, opts.Force); err != nil {
		return err
	}

	g := gdgit.New(a.Root)
	if _, err := g.Run("checkout", ref); err != nil {
		return err
	}

	// Config or manifest may have changed after git checkout, so reload the app.
	after, err := app.Load(ctx, a.Root)
	if err != nil {
		return err
	}
	if err := syncBlobs(ctx, after, opts.Force); err != nil {
		return err
	}
	fmt.Printf("Checked out %s and synchronized blob-managed files\n", ref)
	return nil
}

func removeCurrentKnownBlobFiles(ctx context.Context, a *app.App, force bool) error {
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range m.Entries {
		if e.Deleted || e.Storage != manifest.StorageBlob || e.SHA256 == "" {
			continue
		}
		abs, err := workspace.SafeJoin(a.Root, e.Path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		sha, err := blob.SHA256File(abs)
		if err != nil {
			return err
		}
		if sha != e.SHA256 {
			if err := protectKnownOrForce(ctx, a.Store, abs, e.Path, sha, force); err != nil {
				return err
			}
		}
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
