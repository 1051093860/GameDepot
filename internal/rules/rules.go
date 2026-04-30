package rules

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

type Mode string

const (
	ModeBlob   Mode = "blob"
	ModeGit    Mode = "git"
	ModeIgnore Mode = "ignore"
	// ModeReview means GameDepot detected the file but no safe rule has been chosen yet.
	// Submit refuses review files unless explicitly allowed.
	ModeReview Mode = "review"
)

type Scope string

const (
	ScopeGlob      Scope = "glob"
	ScopeExact     Scope = "exact"
	ScopeDirectory Scope = "directory"
	ScopeExtension Scope = "extension"
)

type Rule struct {
	ID       string `json:"id,omitempty" yaml:"id,omitempty"`
	Pattern  string `json:"pattern" yaml:"pattern"`
	Mode     Mode   `json:"mode" yaml:"mode"`
	Scope    Scope  `json:"scope,omitempty" yaml:"scope,omitempty"`
	Disabled bool   `json:"disabled,omitempty" yaml:"disabled,omitempty"`

	// Kind is kept for backward compatibility with earlier GameDepot configs and reports.
	// The UE v0.8 rules UI does not expose it, but older configs may still contain it.
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

type Match struct {
	Matched bool `json:"matched"`
	Rule    Rule `json:"rule"`
}

type FileClass struct {
	Path        string `json:"path"`
	Mode        Mode   `json:"mode"`
	Kind        string `json:"kind,omitempty"`
	RulePattern string `json:"rule_pattern,omitempty"`
	RuleID      string `json:"rule_id,omitempty"`
	RuleScope   Scope  `json:"rule_scope,omitempty"`
	Matched     bool   `json:"matched"`
}

func Classify(rel string, rules []Rule) Match {
	rel = normalizeRel(rel)
	relLower := strings.ToLower(rel)

	for _, rule := range rules {
		rule = normalizeRule(rule)
		if rule.Disabled || rule.Pattern == "" {
			continue
		}

		ok := false
		var err error
		switch rule.EffectiveScope() {
		case ScopeExact:
			ok = strings.EqualFold(rule.Pattern, rel)
		case ScopeDirectory:
			dir := strings.TrimSuffix(rule.Pattern, "/")
			if ext := path.Ext(dir); ext != "" {
				dir = path.Dir(dir)
			}
			dir = strings.TrimSuffix(normalizeRel(dir), "/")
			ok = relLower == strings.ToLower(dir) || strings.HasPrefix(relLower, strings.ToLower(dir)+"/")
		case ScopeExtension:
			ext := rule.Pattern
			if e := path.Ext(ext); e != "" {
				ext = e
			}
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			ok = strings.EqualFold(path.Ext(rel), ext)
		default:
			ok, err = GlobMatch(rule.Pattern, rel)
			if err != nil {
				continue
			}
		}
		if !ok {
			continue
		}
		return Match{Matched: true, Rule: rule}
	}

	return Match{}
}

func (r Rule) EffectiveScope() Scope {
	if r.Scope == "" {
		return ScopeGlob
	}
	return r.Scope
}

func FileClassFor(rel string, ruleSet []Rule) FileClass {
	rel = normalizeRel(rel)
	match := Classify(rel, ruleSet)
	if !match.Matched {
		return FileClass{Path: rel, Mode: ModeReview, Kind: "unmatched", Matched: false}
	}
	return FileClass{
		Path:        rel,
		Mode:        match.Rule.Mode,
		Kind:        match.Rule.Kind,
		RulePattern: match.Rule.Pattern,
		RuleID:      match.Rule.ID,
		RuleScope:   match.Rule.EffectiveScope(),
		Matched:     true,
	}
}

func ValidateMode(mode Mode) error {
	switch mode {
	case ModeBlob, ModeGit, ModeIgnore, ModeReview:
		return nil
	default:
		return fmt.Errorf("unsupported rule mode %q", mode)
	}
}

func ValidateScope(scope Scope) error {
	switch scope {
	case "", ScopeGlob, ScopeExact, ScopeDirectory, ScopeExtension:
		return nil
	default:
		return fmt.Errorf("unsupported rule scope %q", scope)
	}
}

func ValidateRules(ruleSet []Rule) error {
	if len(ruleSet) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	for i, rule := range ruleSet {
		rule = normalizeRule(rule)
		if rule.Pattern == "" {
			return fmt.Errorf("rules[%d].pattern is required", i)
		}
		if err := ValidateMode(rule.Mode); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if err := ValidateScope(rule.Scope); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if rule.EffectiveScope() == ScopeGlob {
			if _, err := regexp.Compile("^" + globToRegex(rule.Pattern) + "$"); err != nil {
				return fmt.Errorf("rules[%d].pattern %q is invalid: %w", i, rule.Pattern, err)
			}
		}
	}

	return nil
}

func GlobMatch(pattern string, rel string) (bool, error) {
	pattern = normalizeRel(pattern)
	rel = normalizeRel(rel)

	re, err := regexp.Compile("(?i)^" + globToRegex(pattern) + "$")
	if err != nil {
		return false, err
	}

	return re.MatchString(rel), nil
}

func normalizeRule(rule Rule) Rule {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Pattern = normalizeRel(rule.Pattern)
	rule.Mode = Mode(strings.TrimSpace(string(rule.Mode)))
	rule.Scope = Scope(strings.TrimSpace(string(rule.Scope)))
	rule.Kind = strings.TrimSpace(rule.Kind)

	return rule
}

func normalizeRel(v string) string {
	v = strings.ReplaceAll(v, "\\", "/")
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "/")
	return v
}

func globToRegex(pattern string) string {
	var b strings.Builder

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]

		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// Treat **/ as zero or more directories, and bare ** as any chars.
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}

	return b.String()
}
