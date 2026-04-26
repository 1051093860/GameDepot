package manifest

import (
	"encoding/json"
	"os"
)

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}

	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}

	return m, nil
}
