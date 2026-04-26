package commands

import (
	"context"
	"fmt"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Verify(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}

	checked := 0
	deleted := 0
	problems := 0

	for rel, e := range m.Entries {
		if _, err := workspace.CleanRelPath(rel); err != nil {
			fmt.Printf("[error] unsafe manifest path: %q: %v\n", rel, err)
			problems++
			continue
		}
		if e.Path != rel {
			fmt.Printf("[error] entry key/path mismatch: key=%q entry.path=%q\n", rel, e.Path)
			problems++
		}
		if e.Deleted {
			deleted++
			continue
		}
		if e.SHA256 == "" {
			fmt.Printf("[error] missing sha256: %s\n", rel)
			problems++
			continue
		}

		ok, err := a.Store.Has(ctx, e.SHA256)
		if err != nil {
			fmt.Printf("[error] store check failed: %s: %v\n", rel, err)
			problems++
			continue
		}
		if !ok {
			fmt.Printf("[error] missing blob: %s %s\n", shortSHA(e.SHA256), rel)
			problems++
			continue
		}

		r, err := a.Store.Get(ctx, e.SHA256)
		if err != nil {
			fmt.Printf("[error] read blob failed: %s %s: %v\n", shortSHA(e.SHA256), rel, err)
			problems++
			continue
		}
		actual, hashErr := blob.SHA256Reader(r)
		closeErr := r.Close()
		if hashErr != nil {
			fmt.Printf("[error] hash blob failed: %s: %v\n", rel, hashErr)
			problems++
			continue
		}
		if closeErr != nil {
			fmt.Printf("[error] close blob failed: %s: %v\n", rel, closeErr)
			problems++
			continue
		}
		if actual != e.SHA256 {
			fmt.Printf("[error] blob hash mismatch: %s want %s got %s\n", rel, e.SHA256, actual)
			problems++
			continue
		}

		checked++
	}

	if problems > 0 {
		return fmt.Errorf("verify failed: %d problem(s), %d blob(s) checked, %d deleted entry(s)", problems, checked, deleted)
	}

	fmt.Printf("Verify OK: %d blob(s) checked, %d deleted entry(s) skipped\n", checked, deleted)
	return nil
}
