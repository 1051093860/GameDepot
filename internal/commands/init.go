package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/manifest"
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
		fmt.Println("GameDepot already initialized")
		fmt.Println("  config:", filepath.ToSlash(config.ConfigRelPath))
		fmt.Println("  gitignore: ensured UE5/GameDepot rules")
		return nil
	}

	cfg := config.UE5Config(projectID)
	dirs := []string{
		".gamedepot/cache",
		".gamedepot/tmp",
		".gamedepot/logs",
		".gamedepot/runtime",
		".gamedepot/remote_blobs",
		"depot/manifests",
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
	fmt.Println("GameDepot initialized for Unreal project")
	fmt.Println("  project: ", cfg.ProjectID)
	fmt.Println("  config:  ", filepath.ToSlash(config.ConfigRelPath))
	fmt.Println("  manifest:", filepath.ToSlash(cfg.ManifestPath))
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

# GameDepot blob-routed Content binaries
# These files are restored from depot/manifests/*.gdmanifest.json + object storage.
Content/**/*.uasset
Content/**/*.umap
Content/**/*.ubulk
Content/**/*.uexp
Content/**/*.uptnl

# Optional large imported media/sources under Content, also routed by GameDepot rules.
Content/**/*.wav
Content/**/*.ogg
Content/**/*.mp3
Content/**/*.flac
Content/**/*.mp4
Content/**/*.mov
Content/**/*.avi
Content/**/*.png
Content/**/*.jpg
Content/**/*.jpeg
Content/**/*.tga
Content/**/*.exr
Content/**/*.hdr
Content/**/*.fbx
Content/**/*.obj
Content/**/*.gltf
Content/**/*.glb
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
