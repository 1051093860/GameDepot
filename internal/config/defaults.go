package config

import "github.com/1051093860/gamedepot/internal/rules"

func DefaultConfig(projectID string) Config {
	if projectID == "" {
		projectID = "my-game"
	}

	return Config{
		ProjectID:    projectID,
		ManifestPath: "depot/manifests/main.gdmanifest.json",
		Store: StoreConfig{
			Type: "local",
			Root: ".gamedepot/remote_blobs",
		},
		Include: []string{
			"Content/**/*.uasset",
			"Content/**/*.umap",
			"External/**/*",
		},
		Exclude: []string{
			".git/**",
			".gamedepot/cache/**",
			".gamedepot/tmp/**",
			".gamedepot/logs/**",
			".gamedepot/remote_blobs/**",
			"Binaries/**",
			"DerivedDataCache/**",
			"Intermediate/**",
			"Saved/**",
		},
		Rules: []rules.Rule{
			{Pattern: ".git/**", Mode: rules.ModeIgnore, Kind: "runtime"},
			{Pattern: ".gamedepot/cache/**", Mode: rules.ModeIgnore, Kind: "runtime"},
			{Pattern: ".gamedepot/tmp/**", Mode: rules.ModeIgnore, Kind: "runtime"},
			{Pattern: ".gamedepot/logs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
			{Pattern: ".gamedepot/remote_blobs/**", Mode: rules.ModeIgnore, Kind: "runtime"},
			{Pattern: "Binaries/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
			{Pattern: "DerivedDataCache/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
			{Pattern: "Intermediate/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},
			{Pattern: "Saved/**", Mode: rules.ModeIgnore, Kind: "unreal_generated"},

			{Pattern: "Content/**/*.uasset", Mode: rules.ModeBlob, Kind: "unreal_asset"},
			{Pattern: "Content/**/*.umap", Mode: rules.ModeBlob, Kind: "unreal_map"},
			{Pattern: "External/Planning/**/*.xlsx", Mode: rules.ModeBlob, Kind: "planning_excel"},
			{Pattern: "External/Planning/**/*.xls", Mode: rules.ModeBlob, Kind: "planning_excel"},
			{Pattern: "External/Planning/**/*.csv", Mode: rules.ModeBlob, Kind: "planning_table"},
			{Pattern: "External/Planning/**/*.txt", Mode: rules.ModeBlob, Kind: "planning_note"},
			{Pattern: "External/Art/source/**", Mode: rules.ModeBlob, Kind: "art_source"},
			{Pattern: "External/SharedTools/**/*.zip", Mode: rules.ModeBlob, Kind: "tool_package"},
			{Pattern: "External/SharedTools/**/*.7z", Mode: rules.ModeBlob, Kind: "tool_package"},

			{Pattern: "External/WebLinks/**/*.url", Mode: rules.ModeGit, Kind: "shortcut"},
			{Pattern: "External/WebLinks/**/*.md", Mode: rules.ModeGit, Kind: "document"},
			{Pattern: "External/Tech/**/*.py", Mode: rules.ModeGit, Kind: "script"},
			{Pattern: "External/Tech/**/*.ps1", Mode: rules.ModeGit, Kind: "script"},
			{Pattern: "External/Tech/**/*.md", Mode: rules.ModeGit, Kind: "document"},
			{Pattern: "External/Launchers/**/*.bat", Mode: rules.ModeGit, Kind: "launcher"},
			{Pattern: "External/Launchers/**/*.ps1", Mode: rules.ModeGit, Kind: "launcher"},
			{Pattern: "External/**/*.md", Mode: rules.ModeGit, Kind: "document"},
			{Pattern: "External/**/*.url", Mode: rules.ModeGit, Kind: "shortcut"},
		},
	}
}
