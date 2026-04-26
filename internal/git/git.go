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

func (g Git) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.Root

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %v failed: %w\n%s", args, err, stderr.String())
	}

	return stdout.String(), nil
}
