package workspace

import (
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
)

type Classification struct {
	Path        string     `json:"path"`
	IsDir       bool       `json:"is_dir"`
	Mode        rules.Mode `json:"mode"`
	Kind        string     `json:"kind,omitempty"`
	RulePattern string     `json:"rule_pattern,omitempty"`
	Matched     bool       `json:"matched"`
}

func ClassifyRel(rel string, cfg config.Config) (Classification, error) {
	_ = cfg // rules are kept only for backward-compatible config parsing.
	clean, err := CleanRelPath(rel)
	if err != nil {
		return Classification{}, err
	}
	if IsGameDepotManagedPath(clean) {
		return Classification{Path: clean, Mode: rules.ModeBlob, Kind: "content_asset", Matched: true}, nil
	}
	return Classification{Path: clean, Mode: rules.ModeGit, Kind: "git_native", Matched: false}, nil
}

func ClassifyWalk(root string, cfg config.Config, targetRel string, includeUnmatched bool) ([]Classification, error) {
	start := root
	if targetRel != "" {
		joined, err := SafeJoin(root, targetRel)
		if err != nil {
			return nil, err
		}
		start = joined
	}

	out := []Classification{}
	err := filepath.WalkDir(start, func(path string, d fs.DirEntry, walkErr error) error {
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

		if d.IsDir() && ShouldSkipDir(rel, cfg) {
			return filepath.SkipDir
		}

		class, err := ClassifyRel(rel, cfg)
		if err != nil {
			return err
		}
		class.IsDir = d.IsDir()

		if includeUnmatched || class.Matched || class.Mode != rules.ModeIgnore {
			out = append(out, class)
		}

		if d.IsDir() && class.Matched && class.Mode == rules.ModeIgnore {
			return filepath.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
