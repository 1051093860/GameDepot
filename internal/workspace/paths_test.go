package workspace

import "testing"

func TestCleanRelPathRejectsUnsafe(t *testing.T) {
	bad := []string{"../x", "a/../../x", "/tmp/x", "C:/tmp/x", `C:\tmp\x`, `\\server\share\x`, ""}
	for _, p := range bad {
		if got, err := CleanRelPath(p); err == nil {
			t.Fatalf("CleanRelPath(%q)=%q, want error", p, got)
		}
	}
}

func TestCleanRelPathNormalizes(t *testing.T) {
	got, err := CleanRelPath(`External\Planning\a.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "External/Planning/a.txt" {
		t.Fatalf("got %q", got)
	}
}
