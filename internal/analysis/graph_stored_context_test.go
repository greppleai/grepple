package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func TestStoredGraphContextAndFilterFences(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(directory, "cache"))
	sources := []Source{{Path: filepath.Join(directory, "src", "example.ts"), Content: []byte("export function root(){}")}}
	before, _ := storedGraphKey(sources, 0, navigation.NavigationGraphFilter{})
	if err := os.WriteFile(filepath.Join(directory, "tsconfig.json"), []byte(`{"compilerOptions":{"baseUrl":"."}}`), 0600); err != nil {
		t.Fatal(err)
	}
	after, _ := storedGraphKey(sources, 0, navigation.NavigationGraphFilter{})
	if before == "" || before == after {
		t.Fatal("tsconfig did not fence stored generation")
	}
	filtered, _ := storedGraphKey(sources, 0, navigation.NavigationGraphFilter{Languages: []string{"typescript"}})
	limited, _ := storedGraphKey(sources, 1, navigation.NavigationGraphFilter{})
	if filtered == after || limited == after {
		t.Fatal("filter or file limit did not fence stored view")
	}
	if _, _, err := storedContextDigest(directory); err == nil {
		t.Fatal("context directory accepted")
	}
}
