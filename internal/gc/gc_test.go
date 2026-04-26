package gc

import "testing"

func TestSHAFromBlobKey(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, ok := SHAFromBlobKey("sha256/01/23/" + sha + ".blob")
	if !ok || got != sha {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := SHAFromBlobKey("locks/foo.json"); ok {
		t.Fatalf("non-blob key parsed as blob")
	}
}
