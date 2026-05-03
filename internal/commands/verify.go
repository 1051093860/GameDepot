package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type VerifyOptions struct {
	LocalOnly  bool
	RemoteOnly bool
}

func Verify(ctx context.Context, start string) error {
	return VerifyWithOptions(ctx, start, VerifyOptions{})
}

func VerifyWithOptions(ctx context.Context, start string, opts VerifyOptions) error {
	if opts.LocalOnly && opts.RemoteOnly {
		return fmt.Errorf("--local-only and --remote-only cannot be used together")
	}
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}
	refs, err := gdrefs.NewStore(a.Root).LoadAll()
	if err != nil {
		return err
	}
	checked, warnings, problems := 0, 0, 0
	for _, p := range gdrefs.SortedPaths(refs) {
		r := refs[p]
		sha := gdrefs.SHAFromOID(r.OID)
		if sha == "" {
			fmt.Printf("[error] missing oid: %s\n", p)
			problems++
			continue
		}
		if !opts.LocalOnly {
			ok, err := a.Store.Has(ctx, sha)
			if err != nil {
				fmt.Printf("[error] store check failed: %s: %v\n", p, err)
				problems++
				continue
			}
			if !ok {
				fmt.Printf("[error] missing blob: %s %s\n", shortSHA(sha), p)
				problems++
				continue
			}
		}
		if !opts.RemoteOnly {
			abs, err := workspace.SafeJoin(a.Root, p)
			if err != nil {
				fmt.Printf("[error] unsafe local path: %s: %v\n", p, err)
				problems++
				continue
			}
			localSHA, err := blob.SHA256File(abs)
			if err == nil && localSHA != sha {
				fmt.Printf("[warning] local Content differs from ref: %s local=%s ref=%s\n", p, shortSHA(localSHA), shortSHA(sha))
				warnings++
			}
		}
		checked++
	}
	if !opts.RemoteOnly {
		g := gdgit.New(a.Root)
		if trackedFiles, err := g.LsFiles(); err == nil {
			for _, rel := range trackedFiles {
				if workspace.IsGameDepotManagedPath(rel) {
					fmt.Printf("[error] Content file is tracked by Git instead of depot/refs: %s\n", rel)
					problems++
				}
			}
		} else {
			fmt.Printf("[warning] could not inspect Git tracked files: %v\n", err)
			warnings++
		}
		if status, err := g.StatusPorcelain(); err == nil && strings.TrimSpace(status) != "" {
			fmt.Println("[warning] Git working tree has uncommitted changes:")
			fmt.Print(status)
			if !strings.HasSuffix(status, "\n") {
				fmt.Println()
			}
			warnings++
		}
	}
	if problems > 0 {
		return fmt.Errorf("verify failed: %d problem(s), %d warning(s), %d ref(s) checked", problems, warnings, checked)
	}
	if warnings > 0 {
		fmt.Printf("Verify completed with %d warning(s): %d ref(s) checked\n", warnings, checked)
		return nil
	}
	fmt.Printf("Verify OK: %d ref(s) checked\n", checked)
	return nil
}

func isAllowedTrackedSupportFile(rel string) bool {
	switch rel {
	case ".gitignore", ".gamedepot/config.yaml":
		return true
	default:
		return strings.HasPrefix(rel, gdrefs.RootRel+"/")
	}
}
