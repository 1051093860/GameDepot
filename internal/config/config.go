package config

import "github.com/1051093860/gamedepot/internal/rules"

type Config struct {
	ProjectID    string
	ManifestPath string
	Store        StoreConfig
	Include      []string
	Exclude      []string
	Rules        []rules.Rule
}

// StoreConfig is the per-project store selector.
// v0.3 stores credentials and provider endpoints in the global config.
// The Type/Root fields are kept for backward compatibility with v0.2 projects.
type StoreConfig struct {
	Profile string
	Prefix  string

	// Legacy v0.2 fields.
	Type string
	Root string
}
