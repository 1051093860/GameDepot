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
		// GameDepot runtime and VCS internals. These are protected rules and should stay first.
		{Pattern: ".git/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/cache/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/tmp/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/logs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/runtime/**", Mode: rules.ModeIgnore, Kind: "runtime"},
		{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},

		// GameDepot project metadata stays in Git, but is not written into the asset manifest.
		{Pattern: ".gamedepot/config.yaml", Mode: rules.ModeGit, Kind: "gamedepot_config"},
		{Pattern: "depot/manifests/**", Mode: rules.ModeGit, Kind: "gamedepot_manifest"},
		{Pattern: ".gitignore", Mode: rules.ModeGit, Kind: "git_config"},
		{Pattern: ".gitattributes", Mode: rules.ModeGit, Kind: "git_config"},

		// Unreal generated / local-only directories. Non-Content project files are handled directly by Git.
		{Pattern: "Binaries/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Build/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "DerivedDataCache/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Intermediate/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: "Saved/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
		{Pattern: ".vs/**", Mode: rules.ModeIgnore, Kind: "ide_cache"},

		// GameDepot only routes files under Content/**. Binary UE assets go through the blob store.
		{Pattern: "Content/**/*.uasset", Mode: rules.ModeBlob, Kind: "unreal_asset"},
		{Pattern: "Content/**/*.umap", Mode: rules.ModeBlob, Kind: "unreal_map"},
		{Pattern: "Content/**/*.ubulk", Mode: rules.ModeBlob, Kind: "unreal_bulk"},
		{Pattern: "Content/**/*.uexp", Mode: rules.ModeBlob, Kind: "unreal_export"},
		{Pattern: "Content/**/*.uptnl", Mode: rules.ModeBlob, Kind: "unreal_binary"},

		// Raw media / import sources accidentally or intentionally stored under Content.
		{Pattern: "Content/**/*.wav", Mode: rules.ModeBlob, Kind: "media_audio"},
		{Pattern: "Content/**/*.ogg", Mode: rules.ModeBlob, Kind: "media_audio"},
		{Pattern: "Content/**/*.mp3", Mode: rules.ModeBlob, Kind: "media_audio"},
		{Pattern: "Content/**/*.flac", Mode: rules.ModeBlob, Kind: "media_audio"},
		{Pattern: "Content/**/*.mp4", Mode: rules.ModeBlob, Kind: "media_video"},
		{Pattern: "Content/**/*.mov", Mode: rules.ModeBlob, Kind: "media_video"},
		{Pattern: "Content/**/*.avi", Mode: rules.ModeBlob, Kind: "media_video"},
		{Pattern: "Content/**/*.png", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.jpg", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.jpeg", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.tga", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.exr", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.hdr", Mode: rules.ModeBlob, Kind: "texture_source"},
		{Pattern: "Content/**/*.fbx", Mode: rules.ModeBlob, Kind: "model_source"},
		{Pattern: "Content/**/*.obj", Mode: rules.ModeBlob, Kind: "model_source"},
		{Pattern: "Content/**/*.gltf", Mode: rules.ModeBlob, Kind: "model_source"},
		{Pattern: "Content/**/*.glb", Mode: rules.ModeBlob, Kind: "model_source"},

		// Small text/data files under Content can remain Git-managed.
		{Pattern: "Content/**/*.csv", Mode: rules.ModeGit, Kind: "data_source"},
		{Pattern: "Content/**/*.json", Mode: rules.ModeGit, Kind: "data_source"},
		{Pattern: "Content/**/*.txt", Mode: rules.ModeGit, Kind: "text"},
		{Pattern: "Content/**/*.md", Mode: rules.ModeGit, Kind: "document"},
		{Pattern: "Content/**/*.ini", Mode: rules.ModeGit, Kind: "config"},
	}
}
