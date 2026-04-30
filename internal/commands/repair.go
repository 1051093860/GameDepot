package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func RepairCurrentBlob(ctx context.Context, start, target string) error {
	target, err := workspace.CleanRelPath(target)
	if err != nil {
		return err
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}
	e, ok := m.Entries[target]
	if !ok || e.Deleted || e.Storage != manifest.StorageBlob || e.SHA256 == "" {
		return fmt.Errorf("%s has no current manifest blob", target)
	}
	exists, err := a.Store.Has(ctx, e.SHA256)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("current blob already exists: %s %s\n", shortSHA(e.SHA256), target)
		return nil
	}
	abs, err := workspace.SafeJoin(a.Root, target)
	if err != nil {
		return err
	}
	sha, err := blob.SHA256File(abs)
	if err != nil {
		return fmt.Errorf("local file unavailable for repair: %w", err)
	}
	if sha != e.SHA256 {
		return fmt.Errorf("local file SHA %s does not match manifest current SHA %s; submit the local change instead", shortSHA(sha), shortSHA(e.SHA256))
	}
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := a.Store.Put(ctx, e.SHA256, f); err != nil {
		return err
	}
	fmt.Printf("re-uploaded current blob: %s %s\n", shortSHA(e.SHA256), target)
	return nil
}
