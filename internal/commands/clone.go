package commands

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
	gdgit "github.com/1051093860/gamedepot/internal/git"
)

type CloneOptions struct {
	Branch     string
	RemoteName string
	Project    string
	NoUpdate   bool
}

// Clone clones a Git repository and performs the GameDepot post-clone setup.
// Existing GameDepot repositories are materialized with update. Plain UE
// repositories are initialized with GameDepot. Empty repositories are cloned and
// left ready for the user to create/copy a UE project and run init.
func Clone(ctx context.Context, remoteURL, dest string, opts CloneOptions) (string, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return "", fmt.Errorf("remote URL is required")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required: %w", err)
	}

	if strings.TrimSpace(dest) == "" {
		dest = deriveCloneDir(remoteURL)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absDest); err == nil {
		return "", fmt.Errorf("destination already exists: %s", absDest)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absDest), 0o755); err != nil {
		return "", err
	}

	branch := strings.TrimSpace(opts.Branch)
	cloneArgs := []string{"clone", "-c", "core.autocrlf=false"}
	if branch != "" && remoteBranchExists(remoteURL, branch) {
		cloneArgs = append(cloneArgs, "--branch", branch, "--single-branch")
	}
	cloneArgs = append(cloneArgs, remoteURL, absDest)
	if err := runGitStandalone(filepath.Dir(absDest), cloneArgs...); err != nil {
		return "", err
	}

	g := gdgit.New(absDest)
	remoteName := normalizeRemoteName(opts.RemoteName)
	if remoteName != "origin" && g.HasRemote("origin") && !g.HasRemote(remoteName) {
		if _, err := g.Run("remote", "rename", "origin", remoteName); err != nil {
			return "", err
		}
	}
	if err := SetupGitRemoteAndBranch(absDest, GitRemoteSetupOptions{RemoteName: opts.RemoteName, Branch: branch}); err != nil {
		return "", err
	}
	_, _ = g.Run("config", "core.autocrlf", "false")
	_, _ = g.Run("config", "core.eol", "lf")

	if hasConfig(absDest) {
		if !opts.NoUpdate {
			if err := Update(ctx, absDest, UpdateOptions{}); err != nil {
				return "", fmt.Errorf("post-clone update failed: %w", err)
			}
		}
		fmt.Println("GameDepot repository cloned")
		fmt.Println("  root:", absDest)
		return absDest, nil
	}

	if hasUEProject(absDest) {
		if err := ProjectInitUE(ctx, absDest, ProjectInitUEOptions{Project: opts.Project, Profile: "local", Branch: branch, RemoteName: opts.RemoteName}); err != nil {
			return "", err
		}
		if !opts.NoUpdate {
			// A newly initialized plain UE repository usually has no refs yet;
			// update is still harmless and keeps the clone workflow consistent.
			if err := Update(ctx, absDest, UpdateOptions{}); err != nil {
				return "", fmt.Errorf("post-clone update failed: %w", err)
			}
		}
		fmt.Println("UE repository cloned and initialized with GameDepot")
		fmt.Println("  root:", absDest)
		return absDest, nil
	}

	fmt.Println("Repository cloned")
	fmt.Println("  root:", absDest)
	fmt.Println("  note: no .uproject or GameDepot config found; create/copy a UE project, then run `gamedepot init`.")
	return absDest, nil
}

func hasConfig(root string) bool {
	_, err := os.Stat(filepath.Join(root, config.ConfigRelPath))
	return err == nil
}

func hasUEProject(root string) bool {
	matches, _ := filepath.Glob(filepath.Join(root, "*.uproject"))
	return len(matches) > 0
}

func remoteBranchExists(remoteURL, branch string) bool {
	if strings.TrimSpace(branch) == "" {
		return false
	}
	cmd := exec.Command("git", "ls-remote", "--heads", remoteURL, branch)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func runGitStandalone(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v failed: %w\n%s", args, err, string(out))
	}
	return nil
}

func deriveCloneDir(remote string) string {
	raw := strings.TrimSpace(remote)
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		raw = u.Path
	}
	raw = strings.TrimRight(raw, "/")
	raw = filepath.Base(raw)
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." || raw == string(filepath.Separator) {
		return "GameDepotProject"
	}
	return raw
}
