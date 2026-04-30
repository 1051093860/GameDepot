package config

import "github.com/1051093860/gamedepot/internal/rules"

type Config struct {
	ProjectID    string
	ManifestPath string
	User         UserConfig
	Store        StoreConfig
	Git          GitConfig
	Include      []string
	Exclude      []string
	Rules        []rules.Rule
}

type UserConfig struct {
	Identity string
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

// GitConfig is kept only for backward-compatible parsing of older project configs.
// GameDepot no longer stores Git remotes/upstreams; those belong to Git itself.
type GitConfig struct{}

func DefaultGitConfig() GitConfig { return GitConfig{} }
