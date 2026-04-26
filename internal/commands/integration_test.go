package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubmitVerifySyncProtectsDirtyFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "GameDepot Test")

	if err := Init(root, "test-game"); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "External", "Planning", "test.txt")
	writeFile(t, target, "hello gamedepot")

	if err := Submit(ctx, root, "add test file"); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, root); err != nil {
		t.Fatal(err)
	}

	writeFile(t, target, "dirty local edit")
	if err := Sync(ctx, root, false); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite protection error, got %v", err)
	}

	if err := Sync(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello gamedepot" {
		t.Fatalf("restored data=%q", string(data))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
