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
	mustWrite(t, filepath.Join(root, "Content", "Data", "table.csv"), "id,name\n1,A")
	mustWrite(t, filepath.Join(root, "Saved", "ignored.uasset"), "ignored")

	cfg := config.DefaultConfig("test")
	files, err := Scan(root, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(FilterByMode(files, rules.ModeBlob)) != 2 {
		t.Fatalf("blob files = %+v", FilterByMode(files, rules.ModeBlob))
	}
	gitFiles := FilterByMode(files, rules.ModeGit)
	if len(gitFiles) != 0 {
		t.Fatalf("git files = %+v", gitFiles)
	}
	for _, f := range files {
		if !IsGameDepotManagedPath(f.Path) {
			t.Fatalf("Scan should only return Content-managed files, got %+v", files)
		}
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
