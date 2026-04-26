package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/manifest"
	"github.com/1051093860/gamedepot/internal/store"
)

func Doctor(ctx context.Context, start string) error {
	problems := 0

	if path, err := exec.LookPath("git"); err != nil {
		fmt.Println("[error] git: not found in PATH")
		problems++
	} else {
		fmt.Printf("[ok] git: %s\n", path)
	}

	root, err := config.FindRoot(start)
	if err != nil {
		fmt.Printf("[error] root: %v\n", err)
		return fmt.Errorf("doctor found %d problem(s)", problems+1)
	}
	fmt.Printf("[ok] root: %s\n", root)

	cfg, err := config.Load(root)
	if err != nil {
		fmt.Printf("[error] config: %v\n", err)
		return fmt.Errorf("doctor found %d problem(s)", problems+1)
	}
	fmt.Printf("[ok] config: %s\n", filepath.ToSlash(config.ConfigRelPath))
	fmt.Printf("[ok] rules: %d rule(s)\n", len(cfg.Rules))

	manifestPath := filepath.Join(root, cfg.ManifestPath)
	m, err := manifest.Load(manifestPath)
	if err != nil {
		fmt.Printf("[error] manifest: %v\n", err)
		problems++
	} else {
		fmt.Printf("[ok] manifest: %s (%d entries)\n", filepath.ToSlash(cfg.ManifestPath), len(m.Entries))
	}

	a, err := app.Load(ctx, start)
	if err != nil {
		fmt.Printf("[error] app load: %v\n", err)
		problems++
	} else {
		switch s := a.Store.(type) {
		case *store.LocalBlobStore:
			if err := os.MkdirAll(s.Root, 0o755); err != nil {
				fmt.Printf("[error] local store: %v\n", err)
				problems++
			} else {
				fmt.Printf("[ok] local store: %s\n", filepath.ToSlash(strings.TrimPrefix(s.Root, root+string(filepath.Separator))))
			}
		default:
			fmt.Println("[ok] store: configured")
		}
	}

	if problems > 0 {
		return fmt.Errorf("doctor found %d problem(s)", problems)
	}

	fmt.Println("Doctor OK")
	return nil
}
