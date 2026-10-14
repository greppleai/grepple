package examples

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

// Keep copyable example bounds tied to the actual registered parser projection.
// If a fixture changes, update the examples using fresh search output.
func TestVerifiedScopeExamplesMatchFixtures(t *testing.T) {
	for _, fixture := range []struct {
		path, needle, kind string
		match, start, end  int
	}{
		{"testdata/typescript/sample.ts", "logger.info", "func", 10, 8, 12},
		{"examples/sample-files/Button.tsx", "setPressed(true)", "func", 12, 11, 14},
		{"examples/sample-files/config.json", "timeoutSeconds", "object", 15, 13, 16},
		{"examples/sample-files/values.yaml", "extraFlags", "mapping", 37, 34, 37},
		{"examples/sample-files/guide.md", "minVersion", "section", 11, 9, 12},
	} {
		t.Run(fixture.path, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "..", fixture.path))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(content), "\n")
			if fixture.match > len(lines) || !strings.Contains(lines[fixture.match-1], fixture.needle) {
				t.Fatalf("fixture line %d no longer matches %q", fixture.match, fixture.needle)
			}
			language := parser.ScopeContentLanguageFor(fixture.path)
			roles := parser.LineScopeAnnotations(string(content), language, map[int]bool{fixture.match: true})[fixture.match]
			want := parser.ScopeLineRange{Kind: fixture.kind, StartLine: fixture.start, EndLine: fixture.end}
			if roles.Nearest == nil || *roles.Nearest != want {
				t.Fatalf("current %s projection=%+v; update example @%s:%d-%d", language, roles, fixture.kind, fixture.start, fixture.end)
			}
			selector := parser.ScopeSelector{Kind: fixture.kind, StartLine: fixture.start, EndLine: fixture.end, Exact: true}
			if got, status := parser.ResolveScopeSelector(string(content), language, selector, nil); status != parser.ScopeVerified || got != want {
				t.Fatalf("example token no longer verifies: %+v (%s)", got, status)
			}
		})
	}
}
