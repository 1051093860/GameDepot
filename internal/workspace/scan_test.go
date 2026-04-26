package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
)

func TestScanUsesRules(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "Content", "A.uasset"), "asset")
	mustWrite(t, filepath.Join(root, "External", "Tech", "tool.py"), "print('x')")
	mustWrite(t, filepath.Join(root, "Saved", "ignored.uasset"), "ignored")

	cfg := config.DefaultConfig("test")
	files, err := Scan(root, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(FilterByMode(files, rules.ModeBlob)) != 1 {
		t.Fatalf("blob files = %+v", FilterByMode(files, rules.ModeBlob))
	}
	if len(FilterByMode(files, rules.ModeGit)) != 1 {
		t.Fatalf("git files = %+v", FilterByMode(files, rules.ModeGit))
	}
}

func mustWrite(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
