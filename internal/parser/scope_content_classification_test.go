package parser

import "testing"

func TestScopeContentClassificationLeavesLegacyPathsAlone(t *testing.T) {
	for _, tc := range []struct{ path, legacy, annotated string }{
		{"file.go", "go", "go"}, {"file.md", "markdown", "markdown"},
		{"file.json", "text", "json"}, {"file.YAML", "text", "yaml"},
		{"file.yml", "text", "yaml"}, {"file.tf.json", "text", "text"},
		{"stdin", "text", "text"}, {"file.unknown", "text", "text"},
	} {
		if old := NewParser().LanguageFor(tc.path); old != tc.legacy {
			t.Errorf("%s: legacy=%s, want %s", tc.path, old, tc.legacy)
		}
		if next := ScopeContentLanguageFor(tc.path); next != tc.annotated {
			t.Errorf("%s: annotation=%s, want %s", tc.path, next, tc.annotated)
		}
	}
}
