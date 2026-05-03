package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
)

type ProjectInitStatus struct {
	Initialized  bool   `json:"initialized"`
	HasUEProject bool   `json:"has_ue_project"`
	HasGit       bool   `json:"has_git"`
	HasConfig    bool   `json:"has_config"`
	HasManifest  bool   `json:"has_manifest"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	Root         string `json:"root"`
	ConfigPath   string `json:"config_path"`
	ManifestPath string `json:"manifest_path"`
}

type ProjectInitUEOptions struct {
	Project    string
	Profile    string
	RemoteURL  string
	RemoteName string
	Branch     string
}

func ProjectCheckInit(ctx context.Context, root string, jsonOut bool) error {
	_ = ctx
	st, err := CheckProjectInit(root)
	if err != nil {
		return err
	}
	if jsonOut {
		data, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("status: %s\n", st.Status)
	fmt.Printf("initialized: %t\n", st.Initialized)
	fmt.Printf("has_ue_project: %t\n", st.HasUEProject)
	fmt.Printf("has_git: %t\n", st.HasGit)
	fmt.Printf("has_config: %t\n", st.HasConfig)
	fmt.Printf("has_manifest: %t\n", st.HasManifest)
	fmt.Printf("message: %s\n", st.Message)
	return nil
}

func CheckProjectInit(root string) (ProjectInitStatus, error) {
	absRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return ProjectInitStatus{}, err
	}
	if absRoot == "" {
		absRoot, _ = filepath.Abs(".")
	}
	st := ProjectInitStatus{Root: absRoot}
	if matches, _ := filepath.Glob(filepath.Join(absRoot, "*.uproject")); len(matches) > 0 {
		st.HasUEProject = true
	}
	st.HasGit = pathExists(filepath.Join(absRoot, ".git"))
	st.ConfigPath = filepath.Join(absRoot, config.ConfigRelPath)
	st.HasConfig = pathExists(st.ConfigPath)

	// If config can be loaded, use its manifest_path. Otherwise fall back to the default.
	st.ManifestPath = filepath.Join(absRoot, "depot", "manifests", "main.gdmanifest.json")
	if st.HasConfig {
		if cfg, err := config.Load(absRoot); err == nil && cfg.ManifestPath != "" {
			st.ManifestPath = filepath.Join(absRoot, cfg.ManifestPath)
		}
	}
	st.HasManifest = pathExists(st.ManifestPath)
	st.Initialized = st.HasGit && st.HasConfig && st.HasManifest

	switch {
	case st.Initialized:
		st.Status = "initialized"
		st.Message = "GameDepot is initialized for this Unreal project."
	case !st.HasConfig && st.HasGit:
		st.Status = "not_initialized"
		st.Message = "This is a Git project, but GameDepot is not initialized."
	case !st.HasConfig && !st.HasGit:
		st.Status = "not_initialized"
		st.Message = "This Unreal project is not initialized with GameDepot."
	default:
		st.Status = "incomplete"
		st.Message = "GameDepot configuration appears incomplete."
	}
	return st, nil
}

func ProjectInitUE(ctx context.Context, root string, opts ProjectInitUEOptions) error {
	_ = ctx
	absRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return err
	}
	if absRoot == "" {
		absRoot, _ = filepath.Abs(".")
	}
	if _, err := os.Stat(absRoot); err != nil {
		return err
	}

	project := strings.TrimSpace(opts.Project)
	if project == "" {
		project = inferUEProjectName(absRoot)
	}
	if project == "" {
		project = filepath.Base(absRoot)
	}

	st, err := CheckProjectInit(absRoot)
	if err != nil {
		return err
	}

	g := gdgit.New(absRoot)
	if !st.HasGit {
		if _, err := g.Run("init"); err != nil {
			return err
		}
		// Keep config branch default aligned with new projects. Ignore failures for old Git versions.
		_, _ = g.Run("branch", "-M", "main")
	}

	if !st.HasConfig {
		if err := InitUEExisting(absRoot, project); err != nil {
			return err
		}
	} else if !st.HasManifest {
		// Minimal repair. Pointer-refs projects use depot/refs as a directory;
		// legacy manifest projects use a single manifest JSON file.
		cfg, err := config.Load(absRoot)
		if err != nil {
			return err
		}
		if filepath.ToSlash(cfg.ManifestPath) == gdrefs.RootRel {
			if err := os.MkdirAll(filepath.Join(absRoot, filepath.FromSlash(gdrefs.RootRel)), 0o755); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(absRoot, cfg.ManifestPath)), 0o755); err != nil {
				return err
			}
			if err := manifest.Save(filepath.Join(absRoot, cfg.ManifestPath), manifest.New(cfg.ProjectID)); err != nil {
				return err
			}
		}
	}

	if err := appendGitignore(absRoot); err != nil {
		return err
	}
	if err := appendGitattributes(absRoot); err != nil {
		return err
	}
	if err := SetupGitRemoteAndBranch(absRoot, GitRemoteSetupOptions{
		RemoteURL:  opts.RemoteURL,
		RemoteName: opts.RemoteName,
		Branch:     opts.Branch,
	}); err != nil {
		return err
	}

	profile := strings.TrimSpace(opts.Profile)
	if profile == "" || strings.EqualFold(profile, "auto") {
		profile = chooseDefaultUEProfile()
	}
	if profile != "" {
		if err := ConfigProjectUse(ctx, absRoot, profile); err != nil {
			// Local is always safe as a final fallback for first-time UE initialization.
			if profile != "local" {
				if err2 := ConfigProjectUse(ctx, absRoot, "local"); err2 != nil {
					return fmt.Errorf("set profile %q failed: %w; fallback local failed: %v", profile, err, err2)
				}
			} else {
				return err
			}
		}
	}

	if global, err := config.LoadGlobalConfig(); err == nil {
		if strings.TrimSpace(global.User.Name) != "" {
			_, _ = g.Run("config", "user.name", global.User.Name)
		}
		if strings.TrimSpace(global.User.Email) != "" {
			_, _ = g.Run("config", "user.email", global.User.Email)
		}
	}

	fmt.Println("GameDepot UE project initialized")
	fmt.Println("  project:", project)
	fmt.Println("  root:   ", absRoot)
	fmt.Println("  profile:", profile)
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func inferUEProjectName(root string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "*.uproject"))
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(matches[0]), filepath.Ext(matches[0]))
}

func chooseDefaultUEProfile() string {
	global, err := config.LoadGlobalConfig()
	if err != nil {
		return "local"
	}
	if _, ok := global.Profiles["aliyun-oss"]; ok {
		return "aliyun-oss"
	}
	if strings.TrimSpace(global.DefaultProfile) != "" {
		return global.DefaultProfile
	}
	return "local"
}
