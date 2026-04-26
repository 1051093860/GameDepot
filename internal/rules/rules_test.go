package rules

import "testing"

func TestGlobMatchDoubleStar(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"Content/**/*.uasset", "Content/A.uasset", true},
		{"Content/**/*.uasset", "Content/Props/A.uasset", true},
		{"Content/**/*.uasset", "Content/Props/A.umap", false},
		{"External/Art/source/**", "External/Art/source/hero.blend", true},
		{"External/Art/source/**", "External/Art/export/hero.fbx", false},
	}

	for _, tc := range cases {
		got, err := GlobMatch(tc.pattern, tc.path)
		if err != nil {
			t.Fatalf("GlobMatch(%q, %q): %v", tc.pattern, tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("GlobMatch(%q, %q)=%v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestClassifyFirstMatch(t *testing.T) {
	rules := []Rule{
		{Pattern: "External/tmp/**", Mode: ModeIgnore, Kind: "ignored"},
		{Pattern: "External/**/*.txt", Mode: ModeBlob, Kind: "text_blob"},
	}

	m := Classify("External/tmp/a.txt", rules)
	if !m.Matched || m.Rule.Mode != ModeIgnore {
		t.Fatalf("expected ignore first match, got %+v", m)
	}

	m = Classify("External/Planning/a.txt", rules)
	if !m.Matched || m.Rule.Mode != ModeBlob || m.Rule.Kind != "text_blob" {
		t.Fatalf("expected blob match, got %+v", m)
	}
}
