package commands

import (
	"context"
	"fmt"
	"sort"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/manifest"
)

func List(ctx context.Context, start string, includeDeleted bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}
	m.Normalize()

	paths := make([]string, 0, len(m.Entries))
	for p, e := range m.Entries {
		if e.Deleted && !includeDeleted {
			continue
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)

	if len(paths) == 0 {
		fmt.Println("No GameDepot-managed files in manifest")
		return nil
	}

	fmt.Println("path                                      storage  sha256        size        kind              deleted")
	for _, p := range paths {
		e := m.Entries[p]
		fmt.Printf("%-40s  %-7s  %-12s  %-10d  %-16s  %t\n", e.Path, e.Storage, shortSHA(e.SHA256), e.Size, e.Kind, e.Deleted)
	}

	return nil
}
