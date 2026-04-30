package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/1051093860/gamedepot/internal/config"
	"github.com/1051093860/gamedepot/internal/rules"
	"github.com/1051093860/gamedepot/internal/workspace"
)

type RuleScope string

const (
	RuleScopeExact     RuleScope = "exact"
	RuleScopeDirectory RuleScope = "directory"
	RuleScopeExtension RuleScope = "extension"
	RuleScopeGlob      RuleScope = "glob"
)

type RuleSetOptions struct {
	Paths []string
	Mode  rules.Mode
	Kind  string
	Scope RuleScope
}

type RuleSetResult struct {
	Root    string       `json:"root"`
	Added   []rules.Rule `json:"added"`
	Updated []rules.Rule `json:"updated"`
	Rules   []rules.Rule `json:"rules"`
}

func RulesList(ctx context.Context, start string, jsonOut bool) error {
	_ = ctx
	root, err := config.FindRoot(start)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if jsonOut {
		data, _ := json.MarshalIndent(map[string]any{"root": root, "rules": cfg.Rules}, "", "  ")
		fmt.Println(string(data))
		return nil
	}
	fmt.Println("id          mode    scope      enabled  pattern")
	for _, r := range cfg.Rules {
		id := r.ID
		if id == "" {
			id = "-"
		}
		scope := string(r.Scope)
		if scope == "" {
			scope = "glob"
		}
		en := "yes"
		if r.Disabled {
			en = "no"
		}
		fmt.Printf("%-11s %-7s %-10s %-8s %s\n", id, r.Mode, scope, en, r.Pattern)
	}
	return nil
}

func RulesSet(ctx context.Context, start string, opts RuleSetOptions, jsonOut bool) (RuleSetResult, error) {
	_ = ctx
	if err := validateRuleSetOptions(&opts); err != nil {
		return RuleSetResult{}, err
	}
	root, err := config.FindRoot(start)
	if err != nil {
		return RuleSetResult{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return RuleSetResult{}, err
	}

	added := []rules.Rule{}
	updated := []rules.Rule{}
	for _, p := range opts.Paths {
		patterns, err := patternsForRulePath(root, p, opts.Scope)
		if err != nil {
			return RuleSetResult{}, err
		}
		for _, pattern := range patterns {
			r := rules.Rule{Pattern: pattern, Mode: opts.Mode, Scope: rules.Scope(opts.Scope), Kind: opts.Kind}
			wasUpdate := upsertRule(&cfg.Rules, r)
			if wasUpdate {
				updated = append(updated, r)
			} else {
				added = append(added, r)
			}
		}
	}

	if err := rules.ValidateRules(cfg.Rules); err != nil {
		return RuleSetResult{}, err
	}
	if err := config.Save(root, cfg); err != nil {
		return RuleSetResult{}, err
	}

	res := RuleSetResult{Root: root, Added: added, Updated: updated, Rules: cfg.Rules}
	if jsonOut {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, r := range added {
			fmt.Printf("Added rule:   %-6s %-40s %s\n", r.Mode, r.Pattern, r.Kind)
		}
		for _, r := range updated {
			fmt.Printf("Updated rule: %-6s %-40s %s\n", r.Mode, r.Pattern, r.Kind)
		}
	}
	return res, nil
}

func validateRuleSetOptions(opts *RuleSetOptions) error {
	if len(opts.Paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	if opts.Scope == "" {
		opts.Scope = RuleScopeExact
	}
	switch opts.Scope {
	case RuleScopeExact, RuleScopeDirectory, RuleScopeExtension, RuleScopeGlob:
	default:
		return fmt.Errorf("unsupported rule scope %q; use exact, directory, extension, or glob", opts.Scope)
	}
	switch opts.Mode {
	case rules.ModeBlob, rules.ModeGit, rules.ModeIgnore:
		return nil
	case rules.ModeReview:
		return fmt.Errorf("review is an automatic state, not a user rule mode; use blob, git, or ignore")
	default:
		return rules.ValidateMode(opts.Mode)
	}
}

func patternsForRulePath(root string, input string, scope RuleScope) ([]string, error) {
	rel := strings.TrimSpace(strings.ReplaceAll(input, "\\", "/"))
	if rel == "" {
		return nil, fmt.Errorf("path is required")
	}
	// UE package paths are accepted for Content Browser callers.
	if strings.HasPrefix(rel, "/Game/") {
		rel = strings.TrimPrefix(rel, "/Game/")
		ext := filepath.Ext(rel)
		if ext == "" {
			base := filepath.Join(root, "Content", filepath.FromSlash(rel))
			if fileExists(base + ".umap") {
				ext = ".umap"
			} else {
				ext = ".uasset"
			}
		}
		rel = "Content/" + strings.TrimSuffix(rel, filepath.Ext(rel)) + ext
	}

	clean, err := workspace.CleanRelPath(rel)
	if err != nil {
		return nil, err
	}

	switch scope {
	case RuleScopeExact:
		return []string{clean}, nil
	case RuleScopeGlob:
		return []string{clean}, nil
	case RuleScopeDirectory:
		dir := clean
		if ext := path.Ext(clean); ext != "" {
			dir = path.Dir(clean)
		}
		if dir == "." || dir == "" {
			return nil, fmt.Errorf("cannot create directory rule from %q", input)
		}
		return []string{strings.TrimSuffix(dir, "/") + "/**"}, nil
	case RuleScopeExtension:
		ext := path.Ext(clean)
		if ext == "" {
			return nil, fmt.Errorf("extension scope requires a file path with an extension: %s", input)
		}
		dir := path.Dir(clean)
		if dir == "." || dir == "" {
			return []string{"**/*" + ext}, nil
		}
		return []string{strings.TrimSuffix(dir, "/") + "/**/*" + ext}, nil
	default:
		return nil, fmt.Errorf("unsupported rule scope %q", scope)
	}
}

func upsertRule(ruleSet *[]rules.Rule, r rules.Rule) bool {
	r.Pattern = strings.Trim(strings.ReplaceAll(r.Pattern, "\\", "/"), "/")
	r.Kind = strings.TrimSpace(r.Kind)
	for i, existing := range *ruleSet {
		if strings.EqualFold(existing.Pattern, r.Pattern) && existing.Scope == r.Scope {
			(*ruleSet)[i] = r
			return true
		}
	}
	idx := manualRuleInsertIndex(*ruleSet)
	out := append((*ruleSet)[:idx:idx], append([]rules.Rule{r}, (*ruleSet)[idx:]...)...)
	*ruleSet = out
	return false
}

func manualRuleInsertIndex(ruleSet []rules.Rule) int {
	protected := map[string]struct{}{
		".git/**":                    {},
		".gamedepot/cache/**":        {},
		".gamedepot/tmp/**":          {},
		".gamedepot/logs/**":         {},
		".gamedepot/runtime/**":      {},
		".gamedepot/remote_blobs/**": {},
	}
	idx := 0
	for idx < len(ruleSet) {
		p := strings.Trim(strings.ReplaceAll(ruleSet[idx].Pattern, "\\", "/"), "/")
		if _, ok := protected[p]; !ok {
			break
		}
		idx++
	}
	return idx
}

func defaultManualKind(mode rules.Mode) string {
	switch mode {
	case rules.ModeBlob:
		return "manual_blob"
	case rules.ModeGit:
		return "manual_git"
	case rules.ModeIgnore:
		return "manual_ignore"
	default:
		return "manual"
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func sortRules(ruleSet []rules.Rule) []rules.Rule {
	out := append([]rules.Rule{}, ruleSet...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}

func RulesDelete(ctx context.Context, start string, key string) error {
	_ = ctx
	root, err := config.FindRoot(start)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	idx := findRuleIndex(cfg.Rules, key)
	if idx < 0 {
		return fmt.Errorf("rule not found: %s", key)
	}
	removed := cfg.Rules[idx]
	cfg.Rules = append(cfg.Rules[:idx], cfg.Rules[idx+1:]...)
	if err := config.Save(root, cfg); err != nil {
		return err
	}
	fmt.Printf("Deleted rule: %s %s\n", removed.Mode, removed.Pattern)
	return nil
}

func RulesMove(ctx context.Context, start string, key string, delta int) error {
	_ = ctx
	root, err := config.FindRoot(start)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	idx := findRuleIndex(cfg.Rules, key)
	if idx < 0 {
		return fmt.Errorf("rule not found: %s", key)
	}
	newIdx := idx + delta
	minIdx := manualRuleInsertIndex(cfg.Rules)
	if newIdx < minIdx {
		newIdx = minIdx
	}
	if newIdx >= len(cfg.Rules) {
		newIdx = len(cfg.Rules) - 1
	}
	if newIdx == idx {
		return nil
	}
	cfg.Rules[idx], cfg.Rules[newIdx] = cfg.Rules[newIdx], cfg.Rules[idx]
	return config.Save(root, cfg)
}

func RulesSetEnabled(ctx context.Context, start string, key string, enabled bool) error {
	_ = ctx
	root, err := config.FindRoot(start)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	idx := findRuleIndex(cfg.Rules, key)
	if idx < 0 {
		return fmt.Errorf("rule not found: %s", key)
	}
	cfg.Rules[idx].Disabled = !enabled
	return config.Save(root, cfg)
}

func findRuleIndex(ruleSet []rules.Rule, key string) int {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	for i, r := range ruleSet {
		if r.ID != "" && strings.EqualFold(r.ID, key) {
			return i
		}
		if strings.EqualFold(strings.TrimSpace(r.Pattern), key) {
			return i
		}
	}
	return -1
}
