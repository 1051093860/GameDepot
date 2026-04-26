package rules

import (
	"fmt"
	"regexp"
	"strings"
)

type Mode string

const (
	ModeBlob   Mode = "blob"
	ModeGit    Mode = "git"
	ModeIgnore Mode = "ignore"
)

type Rule struct {
	Pattern string `json:"pattern"`
	Mode    Mode   `json:"mode"`
	Kind    string `json:"kind"`
}

type Match struct {
	Matched bool
	Rule    Rule
}

func Classify(rel string, rules []Rule) Match {
	rel = normalizeRel(rel)

	for _, rule := range rules {
		if rule.Pattern == "" {
			continue
		}

		ok, err := GlobMatch(rule.Pattern, rel)
		if err != nil || !ok {
			continue
		}

		return Match{Matched: true, Rule: normalizeRule(rule)}
	}

	return Match{}
}

func ValidateMode(mode Mode) error {
	switch mode {
	case ModeBlob, ModeGit, ModeIgnore:
		return nil
	default:
		return fmt.Errorf("unsupported rule mode %q", mode)
	}
}

func GlobMatch(pattern string, rel string) (bool, error) {
	pattern = normalizeRel(pattern)
	rel = normalizeRel(rel)

	re, err := regexp.Compile("^" + globToRegex(pattern) + "$")
	if err != nil {
		return false, err
	}

	return re.MatchString(rel), nil
}

func normalizeRule(rule Rule) Rule {
	rule.Pattern = normalizeRel(rule.Pattern)
	rule.Mode = Mode(strings.TrimSpace(string(rule.Mode)))
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
