package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func Save(path string, m Manifest) error {
	m.Touch()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
