package historyindex

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
	gdrefs "github.com/1051093860/gamedepot/internal/refs"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type Item struct {
	Path    string           `json:"path"`
	Commit  string           `json:"commit"`
	Date    string           `json:"date"`
	Message string           `json:"message"`
	Storage manifest.Storage `json:"storage"`
	SHA256  string           `json:"sha256,omitempty"`
	Size    int64            `json:"size,omitempty"`
	Deleted bool             `json:"deleted,omitempty"`
}

type Index struct {
	Items []Item `json:"items"`
}

func Build(g gdgit.Git, manifestPath string) (Index, error) {
	commits, err := g.RevList("--date-order", "HEAD")
	if err != nil {
		return Index{}, err
	}
	idx := Index{}
	for _, c := range commits {
		ci, _ := g.CommitInfo(c)
		if items, ok := legacyManifestItemsAtCommit(g, c, ci, manifestPath); ok {
			idx.Items = append(idx.Items, items...)
			continue
		}
		items, err := pointerRefItemsAtCommit(g, c, ci, manifestPath)
		if err != nil {
			continue
		}
		idx.Items = append(idx.Items, items...)
	}
	sortIndexItems(idx.Items)
	return idx, nil
}

// BuildForPath builds history for a single asset path without walking every commit
// and every pointer ref in the repository. Pointer-ref projects can answer this by
// running git log on the one corresponding .gdref file, which is much faster for UE
// history/restore panels.
func BuildForPath(g gdgit.Git, manifestPath, targetPath string) (Index, error) {
	cleaned, err := workspace.CleanRelPath(targetPath)
	if err != nil {
		return Index{}, err
	}
	if isPointerRefsManifestPath(manifestPath) {
		items, err := pointerRefItemsForPath(g, cleaned)
		if err != nil {
			return Index{}, err
		}
		return Index{Items: items}, nil
	}
	items, err := legacyManifestItemsForPath(g, manifestPath, cleaned)
	if err != nil {
		return Index{}, err
	}
	return Index{Items: items}, nil
}

func legacyManifestItemsAtCommit(g gdgit.Git, commit string, ci gdgit.CommitInfo, manifestPath string) ([]Item, bool) {
	raw, err := g.ShowFileBytes(commit, manifestPath)
	if err != nil {
		return nil, false
	}
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	m.Normalize()
	items := make([]Item, 0, len(m.Entries))
	for p, e := range m.Entries {
		if e.Path == "" {
			e.Path = p
		}
		item := Item{Path: e.Path, Commit: commit, Date: ci.Time, Message: ci.Subject, Storage: e.Storage, SHA256: e.SHA256, Size: e.Size, Deleted: e.Deleted}
		if item.Storage == manifest.StorageGit && !item.Deleted {
			if size, err := g.CatFileSize(commit, e.Path); err == nil {
				item.Size = size
			}
		}
		items = append(items, item)
	}
	return items, true
}

func legacyManifestItemsForPath(g gdgit.Git, manifestPath, targetPath string) ([]Item, error) {
	commits, err := g.LogFile(manifestPath)
	if err != nil {
		return nil, err
	}
	items := []Item{}
	for _, ci := range commits {
		raw, err := g.ShowFileBytes(ci.Hash, manifestPath)
		if err != nil {
			continue
		}
		var m manifest.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		m.Normalize()
		e, ok := m.Get(targetPath)
		if !ok {
			continue
		}
		item := Item{Path: targetPath, Commit: ci.Hash, Date: ci.Time, Message: ci.Subject, Storage: e.Storage, SHA256: e.SHA256, Size: e.Size, Deleted: e.Deleted}
		if item.Storage == manifest.StorageGit && !item.Deleted {
			if size, err := g.CatFileSize(ci.Hash, e.Path); err == nil {
				item.Size = size
			}
		}
		items = append(items, item)
	}
	sortIndexItems(items)
	return dedupeItemsForPath(items, targetPath), nil
}

func pointerRefItemsAtCommit(g gdgit.Git, commit string, ci gdgit.CommitInfo, manifestPath string) ([]Item, error) {
	root := strings.Trim(strings.ReplaceAll(manifestPath, "\\", "/"), "/")
	if root == "" || filepath.Base(root) != "refs" {
		root = gdrefs.RootRel
	}
	out, err := g.Run("ls-tree", "-r", "--name-only", commit, "--", root)
	if err != nil {
		return nil, err
	}
	items := []Item{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		refPath := strings.TrimSpace(line)
		if refPath == "" || !strings.HasSuffix(refPath, ".gdref") {
			continue
		}
		assetPath, err := gdrefs.AssetPathFromRef(refPath)
		if err != nil {
			continue
		}
		raw, err := g.ShowFileBytes(commit, refPath)
		if err != nil {
			continue
		}
		item, ok := pointerRefItemFromBytes(raw, assetPath, commit, ci)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func pointerRefItemsForPath(g gdgit.Git, targetPath string) ([]Item, error) {
	refPath, err := gdrefs.RefPathFor(targetPath)
	if err != nil {
		return nil, err
	}
	commits, err := g.LogFile(refPath)
	if err != nil {
		return nil, err
	}
	items := []Item{}
	for _, ci := range commits {
		raw, err := g.ShowFileBytes(ci.Hash, refPath)
		if err != nil {
			// git log includes the commit that deletes the ref. At that commit,
			// commit:path no longer exists, so record an explicit deleted item.
			items = append(items, Item{
				Path:    targetPath,
				Commit:  ci.Hash,
				Date:    ci.Time,
				Message: ci.Subject,
				Storage: manifest.StorageBlob,
				Deleted: true,
			})
			continue
		}
		item, ok := pointerRefItemFromBytes(raw, targetPath, ci.Hash, ci)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	sortIndexItems(items)
	return dedupeItemsForPath(items, targetPath), nil
}

func pointerRefItemFromBytes(raw []byte, assetPath, commit string, ci gdgit.CommitInfo) (Item, bool) {
	var r gdrefs.AssetRef
	if err := json.Unmarshal(raw, &r); err != nil {
		return Item{}, false
	}
	if r.Path == "" {
		r.Path = assetPath
	}
	return Item{
		Path:    r.Path,
		Commit:  commit,
		Date:    ci.Time,
		Message: ci.Subject,
		Storage: manifest.StorageBlob,
		SHA256:  gdrefs.SHAFromOID(r.OID),
		Size:    r.Size,
	}, true
}

func isPointerRefsManifestPath(manifestPath string) bool {
	root := strings.Trim(strings.ReplaceAll(manifestPath, "\\", "/"), "/")
	return root == gdrefs.RootRel || filepath.Base(root) == "refs"
}

func sortIndexItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Path == items[j].Path {
			return items[i].Date > items[j].Date
		}
		return items[i].Path < items[j].Path
	})
}

func dedupeItemsForPath(items []Item, path string) []Item {
	idx := Index{Items: items}
	return idx.ForPath(path)
}

func (idx Index) ForPath(path string) []Item {
	out := []Item{}
	lastKey := ""
	for _, it := range idx.Items {
		if it.Path != path {
			continue
		}
		key := string(it.Storage) + "|" + it.SHA256 + "|" + itoa64(it.Size) + "|" + boolString(it.Deleted)
		if key == lastKey {
			continue
		}
		lastKey = key
		out = append(out, it)
	}
	return out
}

func (idx Index) LatestByPath() map[string]Item {
	out := map[string]Item{}
	for _, it := range idx.Items {
		if _, ok := out[it.Path]; !ok {
			out[it.Path] = it
		}
	}
	return out
}

func HistoryOnly(idx Index, current manifest.Manifest, local map[string]struct{}) []Item {
	current.Normalize()
	latest := idx.LatestByPath()
	out := []Item{}
	for p, it := range latest {
		if it.Deleted {
			continue
		}
		if e, ok := current.Entries[p]; ok && !e.Deleted {
			continue
		}
		if _, ok := local[p]; ok {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	s := ""
	n := v
	for n > 0 {
		s = string(byte('0'+n%10)) + s
		n /= 10
	}
	return s
}
func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
