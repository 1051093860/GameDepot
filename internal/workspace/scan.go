package workspace

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
)

type FileInfo struct {
	Path      string     `json:"path"`
	AbsPath   string     `json:"-"`
	Size      int64      `json:"size"`
	MTimeUnix int64      `json:"mtime_unix"`
	SHA256    string     `json:"sha256"`
	Kind      string     `json:"kind"`
	Mode      rules.Mode `json:"mode"`
}

func Scan(root string, cfg config.Config) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			if ShouldSkipDir(rel, cfg) {
				return filepath.SkipDir
			}
			return nil
		}

		match := rules.Classify(rel, cfg.Rules)
		if !match.Matched || match.Rule.Mode == rules.ModeIgnore {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sha, err := blob.SHA256File(path)
		if err != nil {
			return err
		}

		files = append(files, FileInfo{
			Path:      rel,
			AbsPath:   path,
			Size:      info.Size(),
			MTimeUnix: info.ModTime().Unix(),
			SHA256:    sha,
			Kind:      match.Rule.Kind,
			Mode:      match.Rule.Mode,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func FilterByMode(files []FileInfo, mode rules.Mode) []FileInfo {
	out := make([]FileInfo, 0, len(files))
	for _, f := range files {
		if f.Mode == mode {
			out = append(out, f)
		}
	}
	return out
}

func Paths(files []FileInfo) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func ShouldSkipDir(rel string, cfg config.Config) bool {
	rel = filepath.ToSlash(rel)
	rel = strings.Trim(rel, "/")

	if rel == "." || rel == "" {
		return false
	}

	parts := strings.Split(rel, "/")
	for _, p := range parts {
		switch p {
		case ".git", "Binaries", "DerivedDataCache", "Intermediate", "Saved":
			return true
		}
	}

	if strings.HasPrefix(rel, ".gamedepot/cache") ||
		strings.HasPrefix(rel, ".gamedepot/tmp") ||
		strings.HasPrefix(rel, ".gamedepot/logs") ||
		strings.HasPrefix(rel, ".gamedepot/remote_blobs") {
		return true
	}

	match := rules.Classify(rel+"/placeholder", cfg.Rules)
	return match.Matched && match.Rule.Mode == rules.ModeIgnore
}
