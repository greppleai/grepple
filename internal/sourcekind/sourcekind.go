// Package sourcekind classifies repository files by usage-oriented path conventions.
package sourcekind

import (
	"path/filepath"
	"strings"
)

// Kind is a language-neutral repository source classification.
type Kind string

const (
	// Production is ordinary application or library source.
	Production Kind = "production"
	// Test is conventionally named test source.
	Test Kind = "test"
	// Fixture is example, sample, or test fixture source.
	Fixture Kind = "fixture"
	// Generated is conventionally named generated source.
	Generated Kind = "generated"
	// Vendor is source owned by a vendored dependency tree.
	Vendor Kind = "vendor"
)

// Classify returns a conservative path-based source classification. Unknown paths
// remain production rather than being silently removed from normal analysis.
func Classify(path, root string) Kind {
	relative := path
	if root != "" {
		if value, err := filepath.Rel(root, path); err == nil {
			relative = value
		}
	}
	relative = strings.ToLower(filepath.ToSlash(relative))
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	for _, part := range parts[:max(0, len(parts)-1)] {
		switch part {
		case "vendor", "node_modules", "third_party", "third-party":
			return Vendor
		case "generated", "gen", "dist", "build", "target":
			return Generated
		case "testdata", "fixture", "fixtures", "examples", "samples":
			return Fixture
		case "test", "tests", "__tests__":
			return Test
		}
	}
	base := filepath.Base(relative)
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.Contains(base, ".test."), strings.Contains(base, ".spec."), strings.HasPrefix(base, "test_"):
		return Test
	case strings.Contains(base, ".generated."), strings.HasSuffix(base, "_generated.go"), strings.HasPrefix(base, "generated_"):
		return Generated
	default:
		return Production
	}
}

// IsProduction reports whether path belongs to the default production source set.
func IsProduction(path, root string) bool { return Classify(path, root) == Production }
