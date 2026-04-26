package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/manifest"
)

func Init(root string, projectID string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(absRoot, config.ConfigRelPath)

	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("GameDepot already initialized: %s", cfgPath)
	}

	cfg := config.DefaultConfig(projectID)

	dirs := []string{
		".gamedepot/cache",
		".gamedepot/tmp",
		".gamedepot/logs",
		".gamedepot/remote_blobs",
		"depot/manifests",
		"External/Planning",
		"External/Art",
		"External/Tech",
		"External/SharedTools",
		"External/WebLinks",
		"External/Launchers",
		"Content",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(absRoot, d), 0o755); err != nil {
			return err
		}
	}

	if err := config.Save(absRoot, cfg); err != nil {
		return err
	}

	m := manifest.New(cfg.ProjectID)

	if err := manifest.Save(filepath.Join(absRoot, cfg.ManifestPath), m); err != nil {
		return err
	}

	if err := appendGitignore(absRoot); err != nil {
		return err
	}

	fmt.Println("GameDepot initialized")
	fmt.Println("  config:   ", filepath.ToSlash(config.ConfigRelPath))
	fmt.Println("  manifest: ", filepath.ToSlash(cfg.ManifestPath))

	return nil
}

func appendGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")

	block := `
# GameDepot runtime
.gamedepot/cache/
.gamedepot/tmp/
.gamedepot/logs/
.gamedepot/remote_blobs/

# Unreal generated
Binaries/
DerivedDataCache/
Intermediate/
Saved/
.vs/

# Large assets managed by GameDepot
Content/**/*.uasset
Content/**/*.umap
External/Planning/**/*.xlsx
External/Planning/**/*.xls
External/Planning/**/*.csv
External/Planning/**/*.txt
External/Art/source/**
External/SharedTools/**/*.zip
External/SharedTools/**/*.7z
`

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(block)
	return err
}
