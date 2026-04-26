package locks

import (
	"context"
	"testing"

	"github.com/1051093860/gamedepot/internal/store"
)

func TestLockUnlockList(t *testing.T) {
	ctx := context.Background()
	s := store.NewLocalBlobStore(t.TempDir())
	m := NewManager("test-project", s)
	id := Identity{Owner: "alice", Host: "pc-a"}

	entry, replaced, err := m.Lock(ctx, "Content/Maps/Main.umap", id, "edit map", false)
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("first lock should not replace")
	}
	if entry.Path != "Content/Maps/Main.umap" || entry.Owner != "alice" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	entries, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d locks, want 1", len(entries))
	}

	if _, _, err := m.Lock(ctx, "Content/Maps/Main.umap", Identity{Owner: "bob", Host: "pc-b"}, "", false); err == nil {
		t.Fatal("second owner lock should fail")
	}

	if _, err := m.Unlock(ctx, "Content/Maps/Main.umap", Identity{Owner: "bob", Host: "pc-b"}, false); err == nil {
		t.Fatal("non-owner unlock should fail")
	}

	if _, err := m.Unlock(ctx, "Content/Maps/Main.umap", id, false); err != nil {
		t.Fatal(err)
	}
	entries, err = m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d locks, want 0", len(entries))
	}
}
