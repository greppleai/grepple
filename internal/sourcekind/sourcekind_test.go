package sourcekind

import "testing"

func TestClassifyRepositorySourcePaths(t *testing.T) {
	tests := map[string]Kind{
		"src/service.go":               Production,
		"src/service_test.go":          Test,
		"src/service.spec.ts":          Test,
		"tests/service.py":             Test,
		"testdata/broken/main.go":      Fixture,
		"examples/sample/main.rs":      Fixture,
		"generated/client.go":          Generated,
		"src/types_generated.go":       Generated,
		"vendor/example/dependency.go": Vendor,
	}
	for path, expected := range tests {
		if actual := Classify(path, "."); actual != expected {
			t.Errorf("Classify(%q)=%q, want %q", path, actual, expected)
		}
	}
}
