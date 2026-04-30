package historyindex

import (
	"encoding/json"
	"sort"

	gdgit "github.com/1051093860/gamedepot/internal/git"
	"github.com/1051093860/gamedepot/internal/manifest"
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
		raw, err := g.ShowFileBytes(c, manifestPath)
		if err == nil {
			var m manifest.Manifest
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			m.Normalize()
			for p, e := range m.Entries {
				if e.Path == "" {
					e.Path = p
				}
				item := Item{Path: e.Path, Commit: c, Date: ci.Time, Message: ci.Subject, Storage: e.Storage, SHA256: e.SHA256, Size: e.Size, Deleted: e.Deleted}
				if item.Storage == manifest.StorageGit && !item.Deleted {
					if size, err := g.CatFileSize(c, e.Path); err == nil {
						item.Size = size
					}
				}
				idx.Items = append(idx.Items, item)
			}
		}
	}
	sort.Slice(idx.Items, func(i, j int) bool {
		if idx.Items[i].Path == idx.Items[j].Path {
			return idx.Items[i].Date > idx.Items[j].Date
		}
		return idx.Items[i].Path < idx.Items[j].Path
	})
	return idx, nil
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
