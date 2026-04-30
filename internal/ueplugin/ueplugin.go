package ueplugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const PluginName = "GameDepotUE"

type InstallOptions struct {
	Project            string
	Overwrite          bool
	EnablePlugin       bool
	WriteProjectConfig bool
	WriteUBT           bool
	LowMemoryUBT       bool
	VerifyAfter        bool
	InitProject        bool
	InitProfile        string
}

type InstallResult struct {
	ProjectRoot       string
	PluginSource      string
	PluginDestination string
	ProjectConfigPath string
	UProjectPath      string
	EnabledInUProject bool
	UBTConfigPath     string
}

func Install(opts InstallOptions) error {
	_, err := InstallWithResult(opts)
	return err
}

func InstallWithResult(opts InstallOptions) (InstallResult, error) {
	if !opts.WriteProjectConfig {
		// Historical behavior was to always write project config. The bool exists so
		// callers can opt out later without changing the command surface. Keep the
		// current default as true when the zero-value options are used.
		opts.WriteProjectConfig = true
	}
	if opts.InitProfile == "" {
		opts.InitProfile = "auto"
	}

	projectRoot, err := cleanProjectRoot(opts.Project)
	if err != nil {
		return InstallResult{}, err
	}
	src, err := findTemplateDir()
	if err != nil {
		return InstallResult{}, err
	}
	dst := filepath.Join(projectRoot, "Plugins", PluginName)
	if _, err := os.Stat(dst); err == nil {
		if !opts.Overwrite {
			return InstallResult{}, fmt.Errorf("plugin already exists: %s (use --overwrite)", dst)
		}
		if err := os.RemoveAll(dst); err != nil {
			return InstallResult{}, err
		}
	}
	if err := copyDir(src, dst); err != nil {
		return InstallResult{}, err
	}

	result := InstallResult{ProjectRoot: projectRoot, PluginSource: src, PluginDestination: dst}
	if opts.WriteProjectConfig {
		cfgPath, err := writeConfig(projectRoot)
		if err != nil {
			return result, err
		}
		result.ProjectConfigPath = cfgPath
	}
	if opts.EnablePlugin {
		uproject, changed, err := enablePluginInUProject(projectRoot)
		if err != nil {
			return result, err
		}
		result.UProjectPath = uproject
		result.EnabledInUProject = changed
	}
	if opts.WriteUBT {
		if err := WriteUBTConfig(projectRoot, opts.LowMemoryUBT); err != nil {
			return result, err
		}
		result.UBTConfigPath = filepath.Join(projectRoot, "Saved", "UnrealBuildTool", "BuildConfiguration.xml")
	}
	if opts.VerifyAfter {
		if err := Verify(projectRoot); err != nil {
			return result, err
		}
	}

	fmt.Println("GameDepotUE plugin installed")
	fmt.Println("  project:", projectRoot)
	fmt.Println("  source: ", src)
	fmt.Println("  plugin: ", dst)
	if result.ProjectConfigPath != "" {
		fmt.Println("  config: ", result.ProjectConfigPath)
	}
	if result.UProjectPath != "" {
		fmt.Println("  uproject plugin enabled:", result.UProjectPath)
	}
	if result.UBTConfigPath != "" {
		fmt.Println("  ubt:    ", result.UBTConfigPath)
	}
	return result, nil
}

func Verify(project string) error {
	projectRoot, err := cleanProjectRoot(project)
	if err != nil {
		return err
	}
	required := []string{
		filepath.Join(projectRoot, "Plugins", PluginName, "GameDepotUE.uplugin"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "GameDepotUE.Build.cs"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Public", "GameDepotUEModule.h"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "GameDepotUEModule.cpp"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "GameDepotMockStatusProvider.cpp"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "SGameDepotStatusPanel.cpp"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "SGameDepotConfigPanel.cpp"),
		filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "SGameDepotHistoryDialog.cpp"),
		filepath.Join(projectRoot, "Config", "DefaultGameDepotUE.ini"),
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing plugin file: %s", p)
		}
	}
	if ok, err := uprojectHasEnabledPlugin(projectRoot); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%s is not enabled in the .uproject Plugins list", PluginName)
	}
	fmt.Println("GameDepotUE plugin installation looks OK")
	return nil
}

func WriteUBTConfig(project string, lowMemory bool) error {
	projectRoot, err := cleanProjectRoot(project)
	if err != nil {
		return err
	}
	dir := filepath.Join(projectRoot, "Saved", "UnrealBuildTool")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := `<?xml version="1.0" encoding="utf-8"?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <BuildConfiguration>
    <MaxParallelActions>1</MaxParallelActions>
    <ProcessorCountMultiplier>0.25</ProcessorCountMultiplier>
    <bAllowUBAExecutor>false</bAllowUBAExecutor>
    <bAllowUBALocalExecutor>false</bAllowUBALocalExecutor>
  </BuildConfiguration>
</Configuration>
`
	if !lowMemory {
		content = `<?xml version="1.0" encoding="utf-8"?>
<Configuration xmlns="https://www.unrealengine.com/BuildConfiguration">
  <BuildConfiguration>
    <bAllowUBAExecutor>false</bAllowUBAExecutor>
    <bAllowUBALocalExecutor>false</bAllowUBALocalExecutor>
  </BuildConfiguration>
</Configuration>
`
	}
	out := filepath.Join(dir, "BuildConfiguration.xml")
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func List(project string) error {
	projectRoot, err := cleanProjectRoot(project)
	if err != nil {
		return err
	}
	root := filepath.Join(projectRoot, "Plugins", PluginName)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(projectRoot, path)
		fmt.Println(rel)
		return nil
	})
}

func Diagnose(project string) error {
	projectRoot, err := cleanProjectRoot(project)
	if err != nil {
		return err
	}
	type check struct {
		Name string
		Path string
	}
	checks := []check{
		{"uplugin", filepath.Join(projectRoot, "Plugins", PluginName, "GameDepotUE.uplugin")},
		{"Build.cs", filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "GameDepotUE.Build.cs")},
		{"module header", filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Public", "GameDepotUEModule.h")},
		{"module source", filepath.Join(projectRoot, "Plugins", PluginName, "Source", "GameDepotUE", "Private", "GameDepotUEModule.cpp")},
		{"project plugin config", filepath.Join(projectRoot, "Config", "DefaultGameDepotUE.ini")},
		{"runtime daemon", filepath.Join(projectRoot, ".gamedepot", "runtime", "daemon.json")},
		{"plugin api log", filepath.Join(projectRoot, "Saved", "Logs", "GameDepotUE_API.log")},
		{"daemon api log", filepath.Join(projectRoot, ".gamedepot", "logs", "ue-api.jsonl")},
	}
	fmt.Println("GameDepotUE diagnose")
	fmt.Println("project:", projectRoot)
	for _, c := range checks {
		if st, err := os.Stat(c.Path); err == nil {
			fmt.Printf("[OK]      %-22s %s (%d bytes)\n", c.Name, c.Path, st.Size())
		} else {
			fmt.Printf("[MISSING] %-22s %s\n", c.Name, c.Path)
		}
	}
	if ok, err := uprojectHasEnabledPlugin(projectRoot); err == nil {
		if ok {
			fmt.Println("[OK]      .uproject enables", PluginName)
		} else {
			fmt.Println("[MISSING] .uproject does not enable", PluginName)
		}
	}
	logMatches, _ := filepath.Glob(filepath.Join(projectRoot, "Saved", "Logs", "*.log"))
	fmt.Println("UE log files:")
	for _, p := range logMatches {
		fmt.Println("  ", p)
	}
	fmt.Println("Tip: in UE Output Log, filter by GameDepot or LogGameDepotUE.")
	return nil
}

func cleanProjectRoot(project string) (string, error) {
	if strings.TrimSpace(project) == "" {
		project = "."
	}
	root, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	matches, _ := filepath.Glob(filepath.Join(root, "*.uproject"))
	if len(matches) == 0 {
		return "", fmt.Errorf("not a UE project directory, no .uproject found: %s", root)
	}
	return root, nil
}

func findTemplateDir() (string, error) {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "plugins", "unreal", PluginName),
			filepath.Join(filepath.Dir(exeDir), "plugins", "unreal", PluginName),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "plugins", "unreal", PluginName),
			filepath.Join(filepath.Dir(wd), "plugins", "unreal", PluginName),
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(filepath.Join(c, "GameDepotUE.uplugin")); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("GameDepotUE template not found near executable or working directory")
}

func writeConfig(projectRoot string) (string, error) {
	exe, _ := os.Executable()
	exe = filepath.ToSlash(exe)
	dir := filepath.Dir(exe)
	daemon := filepath.ToSlash(filepath.Join(dir, "gamedepotd.exe"))
	if _, err := os.Stat(daemon); err != nil {
		daemon = exe
	}
	content := fmt.Sprintf(`[GameDepotUE]
MockMode=false
MaxMockAssets=300
GameDepotExecutable=%s
GameDepotDaemonExecutable=%s
DaemonAddress=
DaemonToken=
DaemonListenAddress=127.0.0.1:0
AutoStartDaemon=True
AutoShutdownDaemon=True
`, exe, daemon)
	cfgDir := filepath.Join(projectRoot, "Config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(cfgDir, "DefaultGameDepotUE.ini")
	return out, os.WriteFile(out, []byte(content), 0o644)
}

func enablePluginInUProject(projectRoot string) (string, bool, error) {
	uproject, err := findUProject(projectRoot)
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(uproject)
	if err != nil {
		return uproject, false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return uproject, false, fmt.Errorf("read .uproject json: %w", err)
	}
	pluginsRaw, _ := doc["Plugins"].([]any)
	changed := false
	found := false
	for _, item := range pluginsRaw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(fmt.Sprint(m["Name"]), PluginName) {
			found = true
			if enabled, ok := m["Enabled"].(bool); !ok || !enabled {
				m["Enabled"] = true
				changed = true
			}
		}
	}
	if !found {
		pluginsRaw = append(pluginsRaw, map[string]any{"Name": PluginName, "Enabled": true})
		doc["Plugins"] = pluginsRaw
		changed = true
	}
	if changed {
		out, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return uproject, false, err
		}
		out = append(out, '\n')
		if err := os.WriteFile(uproject, out, 0o644); err != nil {
			return uproject, false, err
		}
	}
	return uproject, changed, nil
}

func uprojectHasEnabledPlugin(projectRoot string) (bool, error) {
	uproject, err := findUProject(projectRoot)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(uproject)
	if err != nil {
		return false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	pluginsRaw, _ := doc["Plugins"].([]any)
	for _, item := range pluginsRaw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(fmt.Sprint(m["Name"]), PluginName) {
			enabled, _ := m["Enabled"].(bool)
			return enabled, nil
		}
	}
	return false, nil
}

func findUProject(projectRoot string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(projectRoot, "*.uproject"))
	if len(matches) == 0 {
		return "", fmt.Errorf("not a UE project directory, no .uproject found: %s", projectRoot)
	}
	return matches[0], nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o644)
	})
}
