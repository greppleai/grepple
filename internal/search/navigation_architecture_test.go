package search

import (
	"os"
	"strings"
	"testing"
)

func TestSearchDoesNotOwnLanguageNavigationResolvers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "navigation_") && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			switch name {
			case "navigation_diff.go", "navigation_filter.go", "navigation_projection.go", "navigation_query.go", "navigation_stats.go":
				continue
			default:
				t.Fatalf("language resolver %s belongs in the navigation package", name)
			}
		}
	}
	content, err := os.ReadFile("related.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"languageNavigationIndex", "resolveGraphCall", "resolveNavigationCandidates"} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("related.go contains repository resolver %q", forbidden)
		}
	}
}
