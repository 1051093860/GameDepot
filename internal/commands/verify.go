package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/blob"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Verify(ctx context.Context, start string) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	if err := rules.ValidateRules(a.Config.Rules); err != nil {
		return err
	}

	m, err := manifest.Load(a.ManifestPath)
	if err != nil {
		return err
	}

	checked := 0
	deleted := 0
	warnings := 0
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

		class, err := workspace.ClassifyRel(rel, a.Config)
		if err != nil {
			fmt.Printf("[error] cannot classify manifest path: %s: %v\n", rel, err)
			problems++
		} else if class.Mode != rules.ModeBlob {
			fmt.Printf("[error] manifest entry is no longer blob-managed by rules: %s mode=%s rule=%s\n", rel, class.Mode, class.RulePattern)
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

		localPath, err := workspace.SafeJoin(a.Root, rel)
		if err != nil {
			fmt.Printf("[error] unsafe local path: %s: %v\n", rel, err)
			problems++
			continue
		}
		if _, err := os.Stat(localPath); err == nil {
			localSHA, err := blob.SHA256File(localPath)
			if err != nil {
				fmt.Printf("[error] local hash failed: %s: %v\n", rel, err)
				problems++
			} else if localSHA != e.SHA256 {
				fmt.Printf("[warning] local blob differs from manifest: %s local=%s manifest=%s\n", rel, shortSHA(localSHA), shortSHA(e.SHA256))
				warnings++
			}
		} else if err != nil && !os.IsNotExist(err) {
			fmt.Printf("[error] local stat failed: %s: %v\n", rel, err)
			problems++
		}

		checked++
	}

	g := gdgit.New(a.Root)
	trackedFiles, err := g.LsFiles()
	if err != nil {
		fmt.Printf("[warning] could not inspect Git tracked files: %v\n", err)
		warnings++
	} else {
		for _, rel := range trackedFiles {
			class, err := workspace.ClassifyRel(rel, a.Config)
			if err != nil {
				fmt.Printf("[error] unsafe Git-tracked path: %s: %v\n", rel, err)
				problems++
				continue
			}
			switch class.Mode {
			case rules.ModeBlob:
				fmt.Printf("[error] blob-managed file is tracked by Git: %s\n", rel)
				fmt.Printf("        suggestion: git rm --cached -- %s\n", rel)
				problems++
			case rules.ModeIgnore:
				if isAllowedTrackedSupportFile(rel) {
					continue
				}
				fmt.Printf("[warning] ignored/unmatched file is tracked by Git: %s\n", rel)
				warnings++
			}
		}
	}

	files, err := workspace.Scan(a.Root, a.Config)
	if err != nil {
		fmt.Printf("[error] workspace scan failed: %v\n", err)
		problems++
	} else {
		for _, f := range workspace.FilterByMode(files, rules.ModeGit) {
			tracked, err := g.IsTracked(f.Path)
			if err != nil {
				fmt.Printf("[warning] could not inspect Git tracking for %s: %v\n", f.Path, err)
				warnings++
				continue
			}
			if !tracked {
				fmt.Printf("[warning] git-managed file is not tracked yet: %s\n", f.Path)
				warnings++
			}
		}
	}

	status, err := g.StatusPorcelain()
	if err == nil && strings.TrimSpace(status) != "" {
		fmt.Println("[warning] Git working tree has uncommitted changes:")
		fmt.Print(status)
		if !strings.HasSuffix(status, "\n") {
			fmt.Println()
		}
		warnings++
	}

	if problems > 0 {
		return fmt.Errorf("verify failed: %d problem(s), %d warning(s), %d blob(s) checked, %d deleted entry(s)", problems, warnings, checked, deleted)
	}

	if warnings > 0 {
		fmt.Printf("Verify completed with %d warning(s): %d blob(s) checked, %d deleted entry(s) skipped\n", warnings, checked, deleted)
		return nil
	}

	fmt.Printf("Verify OK: %d blob(s) checked, %d deleted entry(s) skipped\n", checked, deleted)
	return nil
}

func isAllowedTrackedSupportFile(rel string) bool {
	switch rel {
	case ".gitignore", ".gamedepot/config.yaml":
		return true
	default:
		return strings.HasPrefix(rel, "depot/manifests/")
	}
}
