package refs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/1051093860/gamedepot/internal/workspace"
)

const RootRel = "depot/refs"

// AssetRef is the Git-tracked pointer for exactly one GameDepot-managed file.
// The real file body lives in the blob store, addressed by OID.
type AssetRef struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
	Storage string `json:"storage"`
	OID     string `json:"oid"`
	Size    int64  `json:"size"`
	Kind    string `json:"kind,omitempty"`
}

func NewAssetRef(assetPath, sha256 string, size int64, kind string) AssetRef {
	return AssetRef{Version: 1, Path: normalizeRel(assetPath), Storage: "blob", OID: EnsureOID(sha256), Size: size, Kind: kind}
}

func EnsureOID(sha string) string {
	sha = strings.TrimSpace(sha)
	if sha == "" || strings.HasPrefix(sha, "sha256:") {
		return sha
	}
	return "sha256:" + sha
}

func SHAFromOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), "sha256:")
}

func RefPathFor(assetPath string) (string, error) {
	clean, err := workspace.CleanRelPath(assetPath)
	if err != nil {
		return "", err
	}
	if !workspace.IsGameDepotManagedPath(clean) {
		return "", fmt.Errorf("path is not GameDepot-managed Content: %s", clean)
	}
	return path.Join(RootRel, clean+".gdref"), nil
}

func AssetPathFromRef(refPath string) (string, error) {
	clean := normalizeRel(refPath)
	prefix := RootRel + "/"
	if !strings.HasPrefix(clean, prefix) || !strings.HasSuffix(clean, ".gdref") {
		return "", fmt.Errorf("not a GameDepot ref path: %s", refPath)
	}
	asset := strings.TrimSuffix(strings.TrimPrefix(clean, prefix), ".gdref")
	if _, err := workspace.CleanRelPath(asset); err != nil {
		return "", err
	}
	return asset, nil
}

type Store struct {
	ProjectRoot string
}

func NewStore(projectRoot string) Store { return Store{ProjectRoot: projectRoot} }

func (s Store) LoadAll() (map[string]AssetRef, error) {
	root := filepath.Join(s.ProjectRoot, filepath.FromSlash(RootRel))
	if info, err := os.Stat(root); os.IsNotExist(err) {
		return map[string]AssetRef{}, nil
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", RootRel)
	}

	out := map[string]AssetRef{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.ProjectRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".gdref") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var r AssetRef
		if err := json.Unmarshal(data, &r); err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		assetFromName, err := AssetPathFromRef(rel)
		if err != nil {
			return err
		}
		if r.Path == "" {
			r.Path = assetFromName
		}
		r.Path = normalizeRel(r.Path)
		if r.Path != assetFromName {
			return fmt.Errorf("ref path mismatch: %s declares path %s", rel, r.Path)
		}
		if r.Version == 0 {
			r.Version = 1
		}
		if r.Storage == "" {
			r.Storage = "blob"
		}
		if r.Storage != "blob" {
			return fmt.Errorf("unsupported ref storage %q in %s", r.Storage, rel)
		}
		if r.OID == "" {
			return fmt.Errorf("missing oid in %s", rel)
		}
		out[r.Path] = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s Store) Save(r AssetRef) error {
	r.Path = normalizeRel(r.Path)
	if r.Version == 0 {
		r.Version = 1
	}
	if r.Storage == "" {
		r.Storage = "blob"
	}
	r.OID = EnsureOID(r.OID)
	refRel, err := RefPathFor(r.Path)
	if err != nil {
		return err
	}
	dst := filepath.Join(s.ProjectRoot, filepath.FromSlash(refRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(dst, data, 0o644)
}

func (s Store) Delete(assetPath string) error {
	refRel, err := RefPathFor(assetPath)
	if err != nil {
		return err
	}
	p := filepath.Join(s.ProjectRoot, filepath.FromSlash(refRel))
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return pruneEmptyDirs(filepath.Dir(p), filepath.Join(s.ProjectRoot, filepath.FromSlash(RootRel)))
}

func SortedPaths(m map[string]AssetRef) []string {
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func normalizeRel(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(strings.TrimSpace(p), "./")
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

func pruneEmptyDirs(dir, stop string) error {
	dir, _ = filepath.Abs(dir)
	stop, _ = filepath.Abs(stop)
	for dir != stop && strings.HasPrefix(dir, stop) {
		if err := os.Remove(dir); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return nil
		}
		dir = filepath.Dir(dir)
	}
	return nil
}
