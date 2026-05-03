package smoke

import (
	"context"
	"os/exec"
	"testing"
)

func TestPointerRefsSmoke(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if err := RunPointerRefs(context.Background(), PointerRefsOptions{
		Workspace: t.TempDir(),
		Clean:     false,
		Keep:      true,
	}); err != nil {
		t.Fatal(err)
	}
}
