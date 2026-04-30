package manifest

import "github.com/1051093860/gamedepot/internal/workspace"

type Diff struct {
	Added     []workspace.FileInfo
	Modified  []workspace.FileInfo
	Deleted   []Entry
	Unchanged []workspace.FileInfo
}

// Compare is kept as a blob-manifest diff helper. In manifest v2 the manifest can
// contain git and blob entries, so this function intentionally compares only
// blob-managed entries. Full submit transitions are handled by commands.Submit.
func Compare(m Manifest, files []workspace.FileInfo) Diff {
	m.Normalize()
	current := map[string]workspace.FileInfo{}

	for _, f := range files {
		current[f.Path] = f
	}

	var d Diff

	for _, f := range files {
		old, ok := m.Entries[f.Path]

		if !ok || old.Deleted || old.Storage != StorageBlob {
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
		if old.Deleted || old.Storage != StorageBlob {
			continue
		}

		if _, ok := current[path]; !ok {
			d.Deleted = append(d.Deleted, old)
		}
	}

	return d
}
