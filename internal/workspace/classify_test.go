package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
)

func TestClassifyRel(t *testing.T) {
	cfg := config.DefaultConfig("test")

	cases := []struct {
		path string
		mode rules.Mode
		kind string
	}{
		{"Content/Maps/Main.umap", rules.ModeBlob, "unreal_map"},
		{"Config/DefaultGame.ini", rules.ModeGit, "git_native"},
		{"Source/Game/GameMode.cpp", rules.ModeGit, "git_native"},
		{"Saved/Logs/Game.log", rules.ModeIgnore, "unreal_generated"},
		{"Random/file.tmp", rules.ModeGit, "git_native"},
	}

	for _, tc := range cases {
		got, err := ClassifyRel(tc.path, cfg)
		if err != nil {
			t.Fatalf("ClassifyRel(%q): %v", tc.path, err)
		}
		if got.Mode != tc.mode || got.Kind != tc.kind {
			t.Fatalf("ClassifyRel(%q)=%+v, want mode=%s kind=%s", tc.path, got, tc.mode, tc.kind)
		}
	}
}

func TestClassifyWalkIncludesMatchedAndSkipsIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	mustWriteClassify(t, filepath.Join(root, "Content", "Hero.uasset"), "asset")
	mustWriteClassify(t, filepath.Join(root, "Config", "DefaultGame.ini"), "[/Script/X]")
	mustWriteClassify(t, filepath.Join(root, "Saved", "Logs", "Game.log"), "ignored")

	got, err := ClassifyWalk(root, config.DefaultConfig("test"), "", false)
	if err != nil {
		t.Fatal(err)
	}

	paths := map[string]rules.Mode{}
	for _, item := range got {
		paths[item.Path] = item.Mode
	}

	if paths["Content/Hero.uasset"] != rules.ModeBlob {
		t.Fatalf("missing blob classification: %+v", got)
	}
	if paths["Config/DefaultGame.ini"] != rules.ModeGit {
		t.Fatalf("missing native git classification: %+v", got)
	}
	if _, ok := paths["Saved/Logs/Game.log"]; ok {
		t.Fatalf("ignored dir should be skipped: %+v", got)
	}
}

func mustWriteClassify(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
