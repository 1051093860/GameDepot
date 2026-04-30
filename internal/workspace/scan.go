package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/blob"
	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
)

type FileInfo struct {
	Path        string     `json:"path"`
	AbsPath     string     `json:"-"`
	Size        int64      `json:"size"`
	MTimeUnix   int64      `json:"mtime_unix"`
	SHA256      string     `json:"sha256"`
	Kind        string     `json:"kind"`
	Mode        rules.Mode `json:"mode"`
	RulePattern string     `json:"rule_pattern,omitempty"`
	Matched     bool       `json:"matched"`
}

type ScanOptions struct {
	// HashFiles controls whether ScanAllWithOptions calculates SHA-256 for every
	// visible file. Full-project UE status refreshes should usually keep this
	// false and hash selected assets on demand; large .uasset/.umap projects can
	// otherwise exceed the editor HTTP timeout.
	HashFiles bool
}

// Scan returns only files that are actively managed by GameDepot (blob or git).
// Review and ignored files are intentionally excluded from this compatibility helper.
func Scan(root string, cfg config.Config) ([]FileInfo, error) {
	files, err := ScanManaged(root, cfg)
	if err != nil {
		return nil, err
	}
	out := make([]FileInfo, 0, len(files))
	for _, f := range files {
		if f.Mode == rules.ModeBlob || f.Mode == rules.ModeGit {
			out = append(out, f)
		}
	}
	return out, nil
}

// ScanAll returns every non-ignored file that GameDepot can see, including review files.
func ScanAll(root string, cfg config.Config) ([]FileInfo, error) {
	return ScanAllWithOptions(root, cfg, ScanOptions{HashFiles: true})
}

// ScanAllWithOptions returns every non-ignored file that GameDepot can see. It
// can skip content hashing for fast UI status refreshes.
func ScanAllWithOptions(root string, cfg config.Config, opts ScanOptions) ([]FileInfo, error) {
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

		class, err := ClassifyRel(rel, cfg)
		if err != nil {
			return err
		}
		if class.Mode == rules.ModeIgnore {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sha := ""
		if opts.HashFiles {
			sha, err = blob.SHA256File(path)
			if err != nil {
				return err
			}
		}

		files = append(files, FileInfo{
			Path:        rel,
			AbsPath:     path,
			Size:        info.Size(),
			MTimeUnix:   info.ModTime().Unix(),
			SHA256:      sha,
			Kind:        class.Kind,
			Mode:        class.Mode,
			RulePattern: class.RulePattern,
			Matched:     class.Matched,
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

func ReviewFiles(files []FileInfo) []FileInfo {
	return FilterByMode(files, rules.ModeReview)
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
		strings.HasPrefix(rel, ".gamedepot/runtime") ||
		strings.HasPrefix(rel, ".gamedepot/remote_blobs") {
		return true
	}

	match := rules.Classify(rel+"/placeholder", cfg.Rules)
	return match.Matched && match.Rule.Mode == rules.ModeIgnore
}

// ScanManaged returns files that participate in GameDepot asset routing. For the
// UE5 template this is Content/** only. Non-Content project files are staged by
// Git directly and should not be hashed or reviewed by GameDepot.
func ScanManaged(root string, cfg config.Config) ([]FileInfo, error) {
	return ScanManagedWithOptions(root, cfg, ScanOptions{HashFiles: true})
}

func ScanManagedWithOptions(root string, cfg config.Config, opts ScanOptions) ([]FileInfo, error) {
	contentRoot := filepath.Join(root, "Content")
	if info, err := os.Stat(contentRoot); os.IsNotExist(err) {
		return []FileInfo{}, nil
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return []FileInfo{}, nil
	}

	var files []FileInfo
	err := filepath.WalkDir(contentRoot, func(path string, d fs.DirEntry, walkErr error) error {
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

		if !IsGameDepotManagedPath(rel) {
			return nil
		}

		class, err := ClassifyRel(rel, cfg)
		if err != nil {
			return err
		}
		if class.Mode == rules.ModeIgnore {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		sha := ""
		if opts.HashFiles {
			sha, err = blob.SHA256File(path)
			if err != nil {
				return err
			}
		}

		files = append(files, FileInfo{
			Path:        rel,
			AbsPath:     path,
			Size:        info.Size(),
			MTimeUnix:   info.ModTime().Unix(),
			SHA256:      sha,
			Kind:        class.Kind,
			Mode:        class.Mode,
			RulePattern: class.RulePattern,
			Matched:     class.Matched,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
