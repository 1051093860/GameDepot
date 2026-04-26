package blob

import (
	"path/filepath"
	"testing"
)

func TestPathForSHA256(t *testing.T) {
	got := PathForSHA256("root", "abcdef")
	want := filepath.Join("root", "sha256", "ab", "cd", "abcdef.blob")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
