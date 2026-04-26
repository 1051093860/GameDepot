package workspace

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func ToSlashRel(path string) string {
	return filepath.ToSlash(path)
}

func CleanRelPath(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.ContainsRune(input, '\x00') {
		return "", fmt.Errorf("path contains NUL byte")
	}

	p := strings.ReplaceAll(input, "\\", "/")
	p = strings.TrimSpace(p)

	if path.IsAbs(p) || isWindowsAbs(p) {
		return "", fmt.Errorf("absolute path is not allowed: %s", input)
	}

	clean := path.Clean(p)
	clean = strings.TrimPrefix(clean, "./")

	if clean == "." || clean == "" {
		return "", fmt.Errorf("path is required")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("path escapes project root: %s", input)
	}

	return clean, nil
}

func SafeJoin(root string, rel string) (string, error) {
	clean, err := CleanRelPath(rel)
	if err != nil {
		return "", err
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(filepath.Join(absRoot, filepath.FromSlash(clean)))
	if err != nil {
		return "", err
	}

	if absTarget != absRoot && !strings.HasPrefix(absTarget, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root: %s", rel)
	}

	return absTarget, nil
}

func isWindowsAbs(p string) bool {
	if len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && p[2] == '/' {
		return true
	}
	return strings.HasPrefix(p, "//")
}
