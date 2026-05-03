package localindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const RelPath = ".gamedepot/state/local-index.json"

type Index struct {
	Version int                  `json:"version"`
	Files   map[string]FileState `json:"files"`
}

type FileState struct {
	BaseOID       string `json:"base_oid"`
	BaseRefCommit string `json:"base_ref_commit,omitempty"`
}

func New() Index { return Index{Version: 1, Files: map[string]FileState{}} }

func Load(root string) (Index, error) {
	p := filepath.Join(root, filepath.FromSlash(RelPath))
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	if idx.Version == 0 {
		idx.Version = 1
	}
	if idx.Files == nil {
		idx.Files = map[string]FileState{}
	}
	return idx, nil
}

func Save(root string, idx Index) error {
	if idx.Version == 0 {
		idx.Version = 1
	}
	if idx.Files == nil {
		idx.Files = map[string]FileState{}
	}
	p := filepath.Join(root, filepath.FromSlash(RelPath))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0o644)
}

func (idx *Index) BaseOID(path string) string {
	idx.ensure()
	return idx.Files[norm(path)].BaseOID
}

func (idx *Index) SetBase(path string, oid string) {
	idx.ensure()
	idx.Files[norm(path)] = FileState{BaseOID: oid}
}

func (idx *Index) Delete(path string) {
	idx.ensure()
	delete(idx.Files, norm(path))
}

func (idx *Index) Paths() []string {
	idx.ensure()
	out := make([]string, 0, len(idx.Files))
	for p := range idx.Files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (idx *Index) ensure() {
	if idx.Version == 0 {
		idx.Version = 1
	}
	if idx.Files == nil {
		idx.Files = map[string]FileState{}
	}
}

func norm(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(strings.TrimSpace(p), "./")
	return strings.Trim(p, "/")
}
