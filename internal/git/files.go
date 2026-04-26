package git

import (
	"strings"
)

func (g Git) LsFiles(paths ...string) ([]string, error) {
	args := []string{"ls-files"}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	out, err := g.Run(args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func (g Git) IsTracked(path string) (bool, error) {
	out, err := g.Run("ls-files", "--", path)
	if err != nil {
		return false, err
	}
	for _, line := range splitLines(out) {
		if line == path {
			return true, nil
		}
	}
	return false, nil
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\\", "/"))
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
