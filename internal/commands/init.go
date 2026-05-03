package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
)

func Init(root string, projectID string) error {
	return InitUEExisting(root, projectID)
}

// InitWithTemplate is kept for old callers; template is ignored because v0.8 only
// initializes existing UE5 projects.
func InitWithTemplate(root string, projectID string, template string) error {
	return InitUEExisting(root, projectID)
}

// InitUEExisting initializes GameDepot in an existing Unreal project. It does not
// create sample Content/External placeholder files and it does not support generic
// templates.
func InitUEExisting(root string, projectID string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if matches, _ := filepath.Glob(filepath.Join(absRoot, "*.uproject")); len(matches) == 0 {
		return fmt.Errorf("gamedepot init must be run in an existing Unreal project directory: %s", absRoot)
	}
	if strings.TrimSpace(projectID) == "" || projectID == "my-game" {
		projectID = inferUEProjectName(absRoot)
		if projectID == "" {
			projectID = filepath.Base(absRoot)
		}
	}

	cfgPath := filepath.Join(absRoot, config.ConfigRelPath)
	if _, err := os.Stat(cfgPath); err == nil {
		if err := appendGitignore(absRoot); err != nil {
			return err
		}
		if err := appendGitattributes(absRoot); err != nil {
			return err
		}
		fmt.Println("GameDepot already initialized")
		fmt.Println("  config:", filepath.ToSlash(config.ConfigRelPath))
		fmt.Println("  gitignore: ensured UE5/GameDepot rules")
		fmt.Println("  gitattributes: ensured pointer refs LF normalization")
		return nil
	}

	cfg := config.UE5Config(projectID)
	dirs := []string{
		".gamedepot/cache",
		".gamedepot/tmp",
		".gamedepot/logs",
		".gamedepot/runtime",
		".gamedepot/remote_blobs",
		"depot/refs",
		".gamedepot/state",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(absRoot, d), 0o755); err != nil {
			return err
		}
	}
	if err := config.Save(absRoot, cfg); err != nil {
		return err
	}
	if err := appendGitignore(absRoot); err != nil {
		return err
	}
	if err := appendGitattributes(absRoot); err != nil {
		return err
	}
	fmt.Println("GameDepot initialized for Unreal project")
	fmt.Println("  project: ", cfg.ProjectID)
	fmt.Println("  config:  ", filepath.ToSlash(config.ConfigRelPath))
	fmt.Println("  refs:    depot/refs")
	return nil
}

func appendGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	const begin = "# BEGIN GameDepot UE5"
	const end = "# END GameDepot UE5"
	block := `
# BEGIN GameDepot UE5
# GameDepot runtime / local cache
.gamedepot/cache/
.gamedepot/tmp/
.gamedepot/logs/
.gamedepot/runtime/
.gamedepot/state/
.gamedepot/remote_blobs/

# Unreal generated / local-only folders
Binaries/
Build/
DerivedDataCache/
Intermediate/
Saved/
.vs/
.idea/

# IDE / compiler local files
*.sln
*.suo
*.user
*.userosscache
*.sdf
*.opensdf
*.VC.db
*.VC.VC.opendb
.vscode/.browse.VC.db*
.vscode/ipch/

# Unreal automation / crash / logs
*.log
*.tmp
*.temp
*.pid

# GameDepot manages all project Content through pointer refs.
# Real Content files are materialized by gamedepot update from depot/refs + object storage.
Content/**
# END GameDepot UE5
`
	oldBytes, _ := os.ReadFile(path)
	old := string(oldBytes)
	if strings.Contains(old, begin) && strings.Contains(old, end) {
		start := strings.Index(old, begin)
		finish := strings.Index(old[start:], end)
		if finish >= 0 {
			finish = start + finish + len(end)
			updated := strings.TrimRight(old[:start], "\r\n") + "\n" + strings.Trim(block, "\n") + "\n" + strings.TrimLeft(old[finish:], "\r\n")
			return os.WriteFile(path, []byte(updated), 0o644)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.TrimSpace(old) != "" && !strings.HasSuffix(old, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(block)
	return err
}

func appendGitattributes(root string) error {
	path := filepath.Join(root, ".gitattributes")
	const begin = "# BEGIN GameDepot"
	const end = "# END GameDepot"
	block := `
# BEGIN GameDepot
# Pointer refs and GameDepot metadata must be stable across Windows/macOS/Linux.
# Without this, Windows core.autocrlf can make .gdref files look locally modified
# after clone, which blocks update/pull even when the user only edited Content.
/depot/refs/**/*.gdref text eol=lf
/.gamedepot/config.yaml text eol=lf
/.gitignore text eol=lf
/.gitattributes text eol=lf
# END GameDepot
`
	oldBytes, _ := os.ReadFile(path)
	old := string(oldBytes)
	if strings.Contains(old, begin) && strings.Contains(old, end) {
		start := strings.Index(old, begin)
		finish := strings.Index(old[start:], end)
		if finish >= 0 {
			finish = start + finish + len(end)
			updated := strings.TrimRight(old[:start], "\r\n") + "\n" + strings.Trim(block, "\n") + "\n" + strings.TrimLeft(old[finish:], "\r\n")
			return os.WriteFile(path, []byte(updated), 0o644)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if strings.TrimSpace(old) != "" && !strings.HasSuffix(old, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(block)
	return err
}
