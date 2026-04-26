package manifest

import "github.com/1051093860/gamedepot/internal/workspace"

type Diff struct {
	Added     []workspace.FileInfo
	Modified  []workspace.FileInfo
	Deleted   []Entry
	Unchanged []workspace.FileInfo
}

func Compare(m Manifest, files []workspace.FileInfo) Diff {
	current := map[string]workspace.FileInfo{}

	for _, f := range files {
		current[f.Path] = f
	}

	var d Diff

	for _, f := range files {
		old, ok := m.Entries[f.Path]

		if !ok || old.Deleted {
			d.Added = append(d.Added, f)
			continue
		}

		if old.SHA256 != f.SHA256 || old.Size != f.Size {
			d.Modified = append(d.Modified, f)
			continue
		}

		d.Unchanged = append(d.Unchanged, f)
	}

	for path, old := range m.Entries {
		if old.Deleted {
			continue
		}

		if _, ok := current[path]; !ok {
			d.Deleted = append(d.Deleted, old)
		}
	}

	return d
}
