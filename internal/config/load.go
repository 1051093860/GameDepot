package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/1051093860/gamedepot/internal/rules"
)

const ConfigRelPath = ".gamedepot/config.yaml"

func Load(root string) (Config, error) {
	path := filepath.Join(root, ConfigRelPath)
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var cfg Config
	section := ""
	currentRule := -1

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, "-") {
			section = strings.TrimSuffix(line, ":")
			currentRule = -1
			continue
		}

		if strings.HasPrefix(line, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(line, "-"))

			switch section {
			case "include":
				cfg.Include = append(cfg.Include, trimConfigValue(item))
			case "exclude":
				cfg.Exclude = append(cfg.Exclude, trimConfigValue(item))
			case "rules":
				cfg.Rules = append(cfg.Rules, rules.Rule{})
				currentRule = len(cfg.Rules) - 1
				if item != "" {
					applyRuleKV(&cfg.Rules[currentRule], item)
				}
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = trimConfigValue(value)

		switch section {
		case "user":
			switch key {
			case "identity":
				cfg.User.Identity = value
			}
		case "store":
			switch key {
			case "profile":
				cfg.Store.Profile = value
			case "prefix":
				cfg.Store.Prefix = value
			case "type":
				cfg.Store.Type = value
			case "root":
				cfg.Store.Root = value
			}
		case "git":
			// Backward compatibility: ignored. Use native Git configuration.
		case "rules":
			if currentRule >= 0 && currentRule < len(cfg.Rules) {
				applyRuleField(&cfg.Rules[currentRule], key, value)
			}
		default:
			switch key {
			case "project_id":
				cfg.ProjectID = value
			case "manifest_path":
				cfg.ManifestPath = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Config{}, err
	}

	if cfg.ProjectID == "" {
		return Config{}, errors.New("project_id is required")
	}
	if cfg.ManifestPath == "" {
		return Config{}, errors.New("manifest_path is required")
	}
	if cfg.Store.Profile == "" && cfg.Store.Type == "" {
		return Config{}, errors.New("store.profile is required")
	}
	if cfg.Store.Profile == "" && cfg.Store.Type == "local" {
		cfg.Store.Profile = "local"
	}
	if cfg.Store.Prefix == "" {
		cfg.Store.Prefix = DefaultStorePrefix(cfg.ProjectID)
	}
	if cfg.Store.Root == "" && cfg.Store.Type == "local" {
		cfg.Store.Root = ".gamedepot/remote_blobs"
	}
	if cfg.User.Identity == "" {
		cfg.User.Identity = defaultUserIdentity()
	}
	if len(cfg.Rules) == 0 {
		defaults := DefaultConfig(cfg.ProjectID)
		cfg.Rules = defaults.Rules
	}
	ensureRuntimeIgnoreRules(&cfg)

	if err := rules.ValidateRules(cfg.Rules); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Save(root string, cfg Config) error {
	path := filepath.Join(root, ConfigRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if cfg.Store.Profile == "" && cfg.Store.Type == "local" {
		cfg.Store.Profile = "local"
	}
	if cfg.Store.Prefix == "" {
		cfg.Store.Prefix = DefaultStorePrefix(cfg.ProjectID)
	}
	ensureRuntimeIgnoreRules(&cfg)

	var b strings.Builder

	fmt.Fprintf(&b, "project_id: %s\n\n", cfg.ProjectID)
	fmt.Fprintf(&b, "manifest_path: %s\n\n", cfg.ManifestPath)

	if cfg.User.Identity == "" {
		cfg.User.Identity = defaultUserIdentity()
	}
	fmt.Fprintf(&b, "user:\n")
	fmt.Fprintf(&b, "  identity: %s\n\n", cfg.User.Identity)

	fmt.Fprintf(&b, "store:\n")
	fmt.Fprintf(&b, "  profile: %s\n", cfg.Store.Profile)
	fmt.Fprintf(&b, "  prefix: %s\n\n", cfg.Store.Prefix)

	fmt.Fprintf(&b, "include:\n")
	for _, v := range cfg.Include {
		fmt.Fprintf(&b, "  - %s\n", v)
	}

	fmt.Fprintf(&b, "\nexclude:\n")
	for _, v := range cfg.Exclude {
		fmt.Fprintf(&b, "  - %s\n", v)
	}

	fmt.Fprintf(&b, "\nrules:\n")
	for _, rule := range cfg.Rules {
		fmt.Fprintf(&b, "  - pattern: %s\n", rule.Pattern)
		fmt.Fprintf(&b, "    mode: %s\n", rule.Mode)
		if rule.Scope != "" {
			fmt.Fprintf(&b, "    scope: %s\n", rule.Scope)
		}
		if rule.ID != "" {
			fmt.Fprintf(&b, "    id: %s\n", rule.ID)
		}
		if rule.Disabled {
			fmt.Fprintf(&b, "    disabled: true\n")
		}
		if rule.Kind != "" {
			fmt.Fprintf(&b, "    kind: %s\n", rule.Kind)
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func FindRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	abs = normalizeStartDir(abs)

	cur := abs
	for {
		if _, err := os.Stat(filepath.Join(cur, ConfigRelPath)); err == nil {
			return cur, nil
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("could not find %s from %s", ConfigRelPath, abs)
		}

		cur = parent
	}
}

// ProjectRootCandidate is a user-facing root detection result.
// FindRoot intentionally requires .gamedepot/config.yaml because command paths that
// load the project must fail early when GameDepot is not initialized. For commands
// such as `gamedepot config path`, however, users often run the tool from an
// Unreal project that has not been initialized yet.
type ProjectRootCandidate struct {
	Root      string
	Marker    string
	HasConfig bool
}

func FindProjectRootCandidate(start string) (ProjectRootCandidate, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ProjectRootCandidate{}, err
	}
	abs = normalizeStartDir(abs)

	cur := abs
	for {
		if _, err := os.Stat(filepath.Join(cur, ConfigRelPath)); err == nil {
			return ProjectRootCandidate{Root: cur, Marker: "gamedepot_config", HasConfig: true}, nil
		}
		if st, err := os.Stat(filepath.Join(cur, ".gamedepot")); err == nil && st.IsDir() {
			return ProjectRootCandidate{Root: cur, Marker: "gamedepot_dir", HasConfig: false}, nil
		}
		if matches, _ := filepath.Glob(filepath.Join(cur, "*.uproject")); len(matches) > 0 {
			return ProjectRootCandidate{Root: cur, Marker: "uproject", HasConfig: false}, nil
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return ProjectRootCandidate{}, fmt.Errorf("could not find GameDepot/UE project root from %s", abs)
		}
		cur = parent
	}
}

func normalizeStartDir(abs string) string {
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return filepath.Dir(abs)
	}
	return abs
}

func applyRuleKV(rule *rules.Rule, item string) {
	key, value, ok := strings.Cut(item, ":")
	if !ok {
		return
	}
	applyRuleField(rule, strings.TrimSpace(key), trimConfigValue(value))
}

func applyRuleField(rule *rules.Rule, key string, value string) {
	switch key {
	case "id":
		rule.ID = value
	case "pattern":
		rule.Pattern = value
	case "mode":
		rule.Mode = rules.Mode(value)
	case "scope":
		rule.Scope = rules.Scope(value)
	case "disabled":
		rule.Disabled = parseBoolDefault(value, false)
	case "kind":
		rule.Kind = value
	}
}

func trimConfigValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, `"'`)
	return v
}

func parseBoolDefault(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return def
	}
}

func fillGitDefaults(g *GitConfig) {}

func defaultUserIdentity() string {
	if v := os.Getenv("GAMEDEPOT_USER"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := os.Getenv("USERNAME"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v := os.Getenv("USER"); strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return "unknown"
}

func ensureRuntimeIgnoreRules(cfg *Config) {
	ensureExclude := func(pattern string) {
		for _, existing := range cfg.Exclude {
			if strings.EqualFold(strings.TrimSpace(existing), pattern) {
				return
			}
		}
		cfg.Exclude = append(cfg.Exclude, pattern)
	}

	ensureRule := func(pattern string) {
		for _, rule := range cfg.Rules {
			if strings.EqualFold(strings.TrimSpace(rule.Pattern), pattern) {
				return
			}
		}
		insertAt := 0
		for insertAt < len(cfg.Rules) {
			rule := cfg.Rules[insertAt]
			if rule.Mode != rules.ModeIgnore || !strings.HasPrefix(strings.TrimSpace(rule.Pattern), ".gamedepot/") && strings.TrimSpace(rule.Pattern) != ".git/**" {
				break
			}
			insertAt++
		}
		newRule := rules.Rule{Pattern: pattern, Mode: rules.ModeIgnore, Kind: "runtime"}
		cfg.Rules = append(cfg.Rules, rules.Rule{})
		copy(cfg.Rules[insertAt+1:], cfg.Rules[insertAt:])
		cfg.Rules[insertAt] = newRule
	}

	ensureExclude(".gamedepot/runtime/**")
	ensureRule(".gamedepot/runtime/**")
}
