package search

import (
	"path/filepath"
	"testing"
)

func assertNoForeignNavigation(t *testing.T, matches []FileMatch, foreign string) {
	t.Helper()
	for _, match := range matches {
		for _, point := range match.Related {
			if filepath.Clean(point.File) == filepath.Clean(foreign) {
				t.Fatal("doc-only repository created a spurious cross-repository connection")
			}
		}
	}
}
