package store

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalBlobStorePutHasGetDelete(t *testing.T) {
	ctx := context.Background()
	s := NewLocalBlobStore(t.TempDir())
	sha := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	ok, err := s.Has(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("blob should not exist yet")
	}

	if err := s.Put(ctx, sha, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}

	ok, err = s.Has(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("blob should exist")
	}

	r, err := s.Get(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q", string(data))
	}

	if err := s.Delete(ctx, sha); err != nil {
		t.Fatal(err)
	}
	ok, err = s.Has(ctx, sha)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("blob should be deleted")
	}
}
