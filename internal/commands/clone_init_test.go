package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1051093860/gamedepot/internal/config"
	gdgit "github.com/1051093860/gamedepot/internal/git"
)

func TestProjectInitConfiguresRemoteAndBranch(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	withTestGlobalConfig(t, tmp)

	remote := filepath.Join(tmp, "remote.git")
	runGitForTest(t, tmp, "init", "--bare", remote)

	project := filepath.Join(tmp, "MyGame")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "MyGame.uproject"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ProjectInitUE(context.Background(), project, ProjectInitUEOptions{
		Project:    "MyGame",
		Profile:    "local",
		RemoteURL:  remote,
		RemoteName: "origin",
		Branch:     "dev-main",
	})
	if err != nil {
		t.Fatalf("ProjectInitUE failed: %v", err)
	}

	g := gdgit.New(project)
	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if branch != "dev-main" {
		t.Fatalf("branch = %q, want dev-main", branch)
	}
	gotRemote, err := g.RemoteURL("origin")
	if err != nil {
		t.Fatalf("remote url: %v", err)
	}
	if gotRemote != remote {
		t.Fatalf("remote url = %q, want %q", gotRemote, remote)
	}
	out, err := g.Run("config", "branch.dev-main.remote")
	if err != nil || strings.TrimSpace(out) != "origin" {
		t.Fatalf("branch remote = %q err=%v", strings.TrimSpace(out), err)
	}
	out, err = g.Run("config", "branch.dev-main.merge")
	if err != nil || strings.TrimSpace(out) != "refs/heads/dev-main" {
		t.Fatalf("branch merge = %q err=%v", strings.TrimSpace(out), err)
	}
}

func TestCloneEmptyRemoteFallsBackToLocalInitialBranch(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	withTestGlobalConfig(t, tmp)

	remote := filepath.Join(tmp, "empty.git")
	runGitForTest(t, tmp, "init", "--bare", remote)

	dest := filepath.Join(tmp, "CloneTarget")
	cloned, err := Clone(context.Background(), remote, dest, CloneOptions{Branch: "main", NoUpdate: true})
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}
	if cloned != dest {
		t.Fatalf("cloned path = %q, want %q", cloned, dest)
	}

	g := gdgit.New(dest)
	branch, err := g.CurrentBranch()
	if err != nil {
		t.Fatalf("current branch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	gotRemote, err := g.RemoteURL("origin")
	if err != nil {
		t.Fatalf("remote url: %v", err)
	}
	if gotRemote != remote {
		t.Fatalf("remote url = %q, want %q", gotRemote, remote)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
}

func withTestGlobalConfig(t *testing.T, tmp string) {
	t.Helper()
	old, had := os.LookupEnv(config.EnvConfigDir)
	cfgDir := filepath.Join(tmp, "global-config")
	if err := os.Setenv(config.EnvConfigDir, cfgDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(config.EnvConfigDir, old)
		} else {
			_ = os.Unsetenv(config.EnvConfigDir)
		}
	})
	if err := config.SaveGlobalConfig(config.GlobalConfig{
		DefaultProfile: "local",
		Profiles: map[string]config.StoreProfile{
			"local": {Type: "local", Path: filepath.Join(tmp, "blob-store")},
		},
		User: config.GlobalUser{Name: "Test", Email: "test@example.com"},
	}); err != nil {
		t.Fatal(err)
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
