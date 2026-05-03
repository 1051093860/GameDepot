package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1051093860/gamedepot/internal/app"
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
	writeFile(t, filepath.Join(root, "TestProject.uproject"), "{}")

	if err := Init(root, "test-game"); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "Content", "Characters", "Hero.uasset")
	writeFile(t, target, "hello gamedepot")
	pluginReadme := filepath.Join(root, "Plugins", "GameDepotUE", "README.md")
	writeFile(t, pluginReadme, "plugin docs go through native git")

	if err := Submit(ctx, root, "add test file"); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, root); err != nil {
		t.Fatal(err)
	}
	if tracked := gitTracked(t, root, "Plugins/GameDepotUE/README.md"); !tracked {
		t.Fatalf("plugin README should be tracked directly by Git")
	}
	if tracked := gitTracked(t, root, "Content/Characters/Hero.uasset"); tracked {
		t.Fatalf("blob-routed Content asset should not be tracked by Git")
	}

	writeFile(t, target, "dirty local edit")
	if err := Sync(ctx, root, false); err != nil {
		t.Fatalf("expected normal sync/update to keep local-only changes, got %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dirty local edit" {
		t.Fatalf("normal sync should preserve local-only edit, got %q", string(data))
	}

	if err := Sync(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello gamedepot" {
		t.Fatalf("restored data=%q", string(data))
	}
}

func gitTracked(t *testing.T, dir string, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--", rel)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out)) == rel
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

func TestAssetStatusReportsUnsubmittedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	ctx := context.Background()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "GameDepot Test")
	writeFile(t, filepath.Join(root, "TestProject.uproject"), "{}")

	if err := Init(root, "test-game"); err != nil {
		t.Fatal(err)
	}

	assetPath := filepath.Join(root, "Content", "Characters", "NewHero.uasset")
	writeFile(t, assetPath, "new binary asset")

	a, err := app.Load(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := ComputeAssetStatuses(ctx, a, "Content/Characters/NewHero.uasset", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one status, got %d: %#v", len(statuses), statuses)
	}
	st := statuses[0]
	if st.ManifestStorage != "none" {
		t.Fatalf("expected no manifest storage for unsubmitted file, got %q", st.ManifestStorage)
	}
	if st.DesiredMode != "blob" || st.Mode != "blob" {
		t.Fatalf("expected unsubmitted .uasset to report desired/mode blob, got desired=%q mode=%q", st.DesiredMode, st.Mode)
	}
	if st.Status != "new" || !st.LocalExists || st.LocalSHA256 == "" {
		t.Fatalf("expected new local file with sha, got status=%q local=%v sha=%q", st.Status, st.LocalExists, st.LocalSHA256)
	}
}
