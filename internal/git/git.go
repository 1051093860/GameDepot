package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

type Git struct {
	Root string
}

func New(root string) Git {
	return Git{Root: root}
}

// Run executes git with an explicit -C <root>. This is intentionally used
// instead of relying on process working directory, because GameDepot commands
// may be called from CLI, daemon, tests, UE, or other launchers.
func (g Git) Run(args ...string) (string, error) {
	out, err := g.RunBytes(args...)
	return string(out), err
}

func (g Git) RunBytes(args ...string) ([]byte, error) {
	fullArgs := append([]string{"-C", g.Root}, args...)
	cmd := exec.Command("git", fullArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %v failed: %w\n%s", args, err, stderr.String())
	}

	return stdout.Bytes(), nil
}
