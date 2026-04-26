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
		ManifestPath: "depot/manifests/main.gdmanifest.json",
		Store: StoreConfig{
			Profile: "local",
			Prefix:  DefaultStorePrefix(projectID),
			Type:    "local",
			Root:    ".gamedepot/remote_blobs",
		},
		Include: []string{"**/*"},
		Exclude: []string{
			".git/**",
			".gamedepot/cache/**",
			".gamedepot/tmp/**",
			".gamedepot/logs/**",
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
		{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
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
		// GameDepot runtime and VCS internals.
		{Pattern: ".git/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/cache/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/tmp/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/logs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},

		// Unreal generated / local-only directories.
		{Pattern: "Binaries/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Build/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "DerivedDataCache/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Intermediate/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Saved/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: ".vs/**", Mode: rules.ModeIgnore, Kind: "ide_cache"},

		// Unreal binary assets go through the blob store.
		{Pattern: "Content/**/*.uasset", Mode: rules.ModeBlob, Kind: "unreal_asset"},
		{Pattern: "Content/**/*.umap", Mode: rules.ModeBlob, Kind: "unreal_map"},

		// Project and source files stay in Git.
		{Pattern: "*.uproject", Mode: rules.ModeGit, Kind: "unreal_project"},
		{Pattern: "Config/**", Mode: rules.ModeGit, Kind: "unreal_config"},
		{Pattern: "Source/**", Mode: rules.ModeGit, Kind: "code"},
		{Pattern: "Plugins/**/*.uplugin", Mode: rules.ModeGit, Kind: "unreal_plugin"},
		{Pattern: "Plugins/**/Source/**", Mode: rules.ModeGit, Kind: "code"},
		{Pattern: "Docs/**", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "README.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "LICENSE", Mode: rules.ModeGit, Kind: "document"},

		// Planning and art source files usually need binary history and restore.
		{Pattern: "External/Planning/**/*.xlsx", Mode: rules.ModeBlob, Kind: "planning_excel"},
		{Pattern: "External/Planning/**/*.xls", Mode: rules.ModeBlob, Kind: "planning_excel"},
		{Pattern: "External/Planning/**/*.csv", Mode: rules.ModeBlob, Kind: "planning_table"},
		{Pattern: "External/Planning/**/*.txt", Mode: rules.ModeBlob, Kind: "planning_note"},
		{Pattern: "External/Art/source/**", Mode: rules.ModeBlob, Kind: "art_source"},
		{Pattern: "External/Art/**/*.psd", Mode: rules.ModeBlob, Kind: "art_source"},
		{Pattern: "External/Art/**/*.blend", Mode: rules.ModeBlob, Kind: "art_source"},
		{Pattern: "External/Art/**/*.fbx", Mode: rules.ModeBlob, Kind: "art_export"},
		{Pattern: "External/Art/**/*.png", Mode: rules.ModeBlob, Kind: "art_export"},
		{Pattern: "External/Art/**/*.jpg", Mode: rules.ModeBlob, Kind: "art_export"},
		{Pattern: "External/Art/**/*.jpeg", Mode: rules.ModeBlob, Kind: "art_export"},
		{Pattern: "External/SharedTools/**/*.zip", Mode: rules.ModeBlob, Kind: "tool_package"},
		{Pattern: "External/SharedTools/**/*.7z", Mode: rules.ModeBlob, Kind: "tool_package"},
		{Pattern: "External/SharedTools/**/*.exe", Mode: rules.ModeBlob, Kind: "tool_binary"},

		// Lightweight external-workspace entries stay in Git.
		{Pattern: "External/WebLinks/**/*.url", Mode: rules.ModeGit, Kind: "shortcut"},
		{Pattern: "External/WebLinks/**/*.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "External/Tech/**/*.py", Mode: rules.ModeGit, Kind: "script"},
		{Pattern: "External/Tech/**/*.ps1", Mode: rules.ModeGit, Kind: "script"},
		{Pattern: "External/Tech/**/*.bat", Mode: rules.ModeGit, Kind: "script"},
		{Pattern: "External/Tech/**/*.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "External/Launchers/**/*.bat", Mode: rules.ModeGit, Kind: "launcher"},
		{Pattern: "External/Launchers/**/*.ps1", Mode: rules.ModeGit, Kind: "launcher"},
		{Pattern: "External/**/*.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "External/**/*.url", Mode: rules.ModeGit, Kind: "shortcut"},
	}
}
