package manifest

import "time"

type Storage string

const (
	StorageGit    Storage = "git"
	StorageBlob   Storage = "blob"
	StorageIgnore Storage = "ignore"
)

type Manifest struct {
	Version   int              `json:"version"`
	ProjectID string           `json:"project_id"`
	UpdatedAt string           `json:"updated_at"`
	Entries   map[string]Entry `json:"entries"`
}

type Entry struct {
	Path      string  `json:"path"`
	Storage   Storage `json:"storage"`
	SHA256    string  `json:"sha256,omitempty"`
	Size      int64   `json:"size,omitempty"`
	MTimeUnix int64   `json:"mtime_unix,omitempty"`
	Kind      string  `json:"kind,omitempty"`
	Deleted   bool    `json:"deleted,omitempty"`
}

func New(projectID string) Manifest {
	return Manifest{
		Version:   2,
		ProjectID: projectID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]Entry{},
	}
}

func (m *Manifest) Touch() {
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if m.Version <= 0 {
		m.Version = 2
	}
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	m.Normalize()
}

func (m *Manifest) Normalize() {
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	if m.Version <= 0 {
		m.Version = 2
	}
	for p, e := range m.Entries {
		if e.Path == "" {
			e.Path = p
		}
		// Backward compatibility: old manifests only listed blob-managed entries and had no Storage field.
		if e.Storage == "" {
			if e.SHA256 != "" {
				e.Storage = StorageBlob
			} else {
				e.Storage = StorageGit
			}
		}
		m.Entries[p] = e
	}
}

func (e Entry) IsBlob() bool {
	return !e.Deleted && e.Storage == StorageBlob && e.SHA256 != ""
}

func (e Entry) IsGit() bool {
	return !e.Deleted && e.Storage == StorageGit
}

func (m *Manifest) Get(path string) (Entry, bool) {
	m.Normalize()
	e, ok := m.Entries[path]
	return e, ok
}

func (m *Manifest) Upsert(e Entry) {
	m.Normalize()
	if e.Path == "" {
		return
	}
	m.Entries[e.Path] = e
}

func (m *Manifest) Remove(path string) {
	m.Normalize()
	delete(m.Entries, path)
}

func (m *Manifest) BlobRefs() []string {
	m.Normalize()
	seen := map[string]struct{}{}
	out := []string{}
	for _, e := range m.Entries {
		if e.IsBlob() && e.SHA256 != "" {
			if _, ok := seen[e.SHA256]; ok {
				continue
			}
			seen[e.SHA256] = struct{}{}
			out = append(out, e.SHA256)
		}
	}
	return out
}
