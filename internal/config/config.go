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

type StoreConfig struct {
	Type string
	Root string
}
