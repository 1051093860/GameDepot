package manifest

import (
	"testing"

	"github.com/1051093860/gamedepot/internal/workspace"
)

func TestCompare(t *testing.T) {
	m := Manifest{Entries: map[string]Entry{
		"same.txt":    {Path: "same.txt", SHA256: "aaa", Size: 3},
		"changed.txt": {Path: "changed.txt", SHA256: "old", Size: 3},
		"deleted.txt": {Path: "deleted.txt", SHA256: "ddd", Size: 3},
	}}

	files := []workspace.FileInfo{
		{Path: "same.txt", SHA256: "aaa", Size: 3},
		{Path: "changed.txt", SHA256: "new", Size: 3},
		{Path: "added.txt", SHA256: "bbb", Size: 3},
	}

	d := Compare(m, files)
	if len(d.Added) != 1 || d.Added[0].Path != "added.txt" {
		t.Fatalf("bad added: %+v", d.Added)
	}
	if len(d.Modified) != 1 || d.Modified[0].Path != "changed.txt" {
		t.Fatalf("bad modified: %+v", d.Modified)
	}
	if len(d.Deleted) != 1 || d.Deleted[0].Path != "deleted.txt" {
		t.Fatalf("bad deleted: %+v", d.Deleted)
	}
	if len(d.Unchanged) != 1 || d.Unchanged[0].Path != "same.txt" {
		t.Fatalf("bad unchanged: %+v", d.Unchanged)
	}
}
