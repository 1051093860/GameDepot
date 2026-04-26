package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/manifest"
)

func Init(root string, projectID string) error {
	return InitWithTemplate(root, projectID, config.DefaultTemplate)
}

func InitWithTemplate(root string, projectID string, template string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(absRoot, config.ConfigRelPath)
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("GameDepot already initialized: %s", cfgPath)
	}

	cfg, err := config.ConfigForTemplate(projectID, template)
	if err != nil {
		return err
	}

	dirs := []string{
		".gamedepot/cache",
		".gamedepot/tmp",
		".gamedepot/logs",
		".gamedepot/remote_blobs",
		"depot/manifests",
		"Content",
		"Config",
		"Source",
		"Docs",
		"External/Planning",
		"External/Art/source",
		"External/Art/export",
		"External/Tech",
		"External/SharedTools",
		"External/WebLinks",
		"External/Launchers",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(absRoot, d), 0o755); err != nil {
			return err
		}
	}

	if err := writeTemplatePlaceholders(absRoot, cfg.ProjectID); err != nil {
		return err
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
	fmt.Println("  template: ", template)
	fmt.Println("  config:   ", filepath.ToSlash(config.ConfigRelPath))
	fmt.Println("  manifest: ", filepath.ToSlash(cfg.ManifestPath))

	return nil
}

func writeTemplatePlaceholders(root string, projectID string) error {
	files := map[string]string{
		"Docs/README.md":               "# " + projectID + "\n\nProject documentation managed by Git.\n",
		"External/WebLinks/README.md":  "# Web Links\n\nPlace .url shortcuts or link notes here.\n",
		"External/Tech/README.md":      "# Tech Tools\n\nPlace small scripts and technical notes here.\n",
		"External/Launchers/README.md": "# Launchers\n\nPlace .bat/.ps1 launchers here.\n",
	}

	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}

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

# Unreal generated / local-only files
Binaries/
Build/
DerivedDataCache/
Intermediate/
Saved/
.vs/

# Large assets managed by GameDepot blob store
Content/**/*.uasset
Content/**/*.umap
External/Planning/**/*.xlsx
External/Planning/**/*.xls
External/Planning/**/*.csv
External/Planning/**/*.txt
External/Art/source/
External/Art/**/*.psd
External/Art/**/*.blend
External/Art/**/*.fbx
External/Art/**/*.png
External/Art/**/*.jpg
External/Art/**/*.jpeg
External/SharedTools/**/*.zip
External/SharedTools/**/*.7z
External/SharedTools/**/*.exe
`

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(block)
	return err
}
