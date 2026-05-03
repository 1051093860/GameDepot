package config

import (
	"fmt"

	"github.com/1051093860/gamedepot/internal/rules"
)

const DefaultTemplate = "ue5"

func DefaultStorePrefix(projectID string) string {
	if projectID == "" {
		projectID = "my-game"
	}
	return "projects/" + projectID + "/blobs"
}

func ConfigForTemplate(projectID string, template string) (Config, error) {
	if projectID == "" {
		projectID = "my-game"
	}
	if template == "" {
		template = DefaultTemplate
	}

	switch template {
	case "ue5", "unreal", "unreal5":
		return UE5Config(projectID), nil
	case "basic", "default":
		return BasicConfig(projectID), nil
	default:
		return Config{}, fmt.Errorf("unsupported template %q; available templates: ue5, basic", template)
	}
}

func DefaultConfig(projectID string) Config {
	return UE5Config(projectID)
}

func baseConfig(projectID string, ruleSet []rules.Rule) Config {
	if projectID == "" {
		projectID = "my-game"
	}

	return Config{
		ProjectID:    projectID,
		ManifestPath: "depot/refs",
		Store: StoreConfig{
			Profile: "local",
			Prefix:  DefaultStorePrefix(projectID),
			Type:    "local",
			Root:    ".gamedepot/remote_blobs",
		},
		Git:     DefaultGitConfig(),
		Include: []string{"**/*"},
		Exclude: []string{
			".git/**",
			".gamedepot/cache/**",
			".gamedepot/tmp/**",
			".gamedepot/logs/**",
			".gamedepot/runtime/**",
			".gamedepot/remote_blobs/**",
		},
		Rules: ruleSet,
	}
}

func BasicConfig(projectID string) Config {
	return baseConfig(projectID, []rules.Rule{
		{Pattern: ".git/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/cache/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/tmp/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/logs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/runtime/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/config.yaml", Mode: rules.ModeGit, Kind: "gamedepot_config"},
		{Pattern: "depot/manifests/**", Mode: rules.ModeGit, Kind: "gamedepot_manifest"},
		{Pattern: "External/Planning/**/*.xlsx", Mode: rules.ModeBlob, Kind: "planning_excel"},
		{Pattern: "External/Planning/**/*.xls", Mode: rules.ModeBlob, Kind: "planning_excel"},
		{Pattern: "External/SharedTools/**/*.zip", Mode: rules.ModeBlob, Kind: "tool_package"},
		{Pattern: "External/**/*.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "External/**/*.url", Mode: rules.ModeGit, Kind: "shortcut"},
		{Pattern: "Docs/**", Mode: rules.ModeGit, Kind: "document"},
	})
}

func UE5Config(projectID string) Config {
	return baseConfig(projectID, DefaultUE5Rules())
}

func DefaultUE5Rules() []rules.Rule {
	return []rules.Rule{
		{Pattern: ".git/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/cache/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/tmp/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/logs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/runtime/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/state/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
	}
}
