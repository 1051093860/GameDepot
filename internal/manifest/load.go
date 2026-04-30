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

	m.Normalize()

	return m, nil
}

// LoadBytes parses a manifest from an arbitrary Git object or cache entry.
func LoadBytes(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	m.Normalize()
	return m, nil
}
