package manifest

import "time"

type Manifest struct {
	Version   int              `json:"version"`
	ProjectID string           `json:"project_id"`
	UpdatedAt string           `json:"updated_at"`
	Entries   map[string]Entry `json:"entries"`
}

type Entry struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	MTimeUnix int64  `json:"mtime_unix"`
	Kind      string `json:"kind"`
	Deleted   bool   `json:"deleted"`
}

func New(projectID string) Manifest {
	return Manifest{
		Version:   1,
		ProjectID: projectID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:   map[string]Entry{},
	}
}

func (m *Manifest) Touch() {
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
}
