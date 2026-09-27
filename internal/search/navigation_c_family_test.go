package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCFamilyQuotedIncludesResolveOnlyExactSourceRelativeTargets(t *testing.T) {
	t.Run("c", func(t *testing.T) { assertCFamilyIncludeResolution(t, ".c") })
	t.Run("cpp", func(t *testing.T) { assertCFamilyIncludeResolution(t, ".cpp") })
}

func assertCFamilyIncludeResolution(t *testing.T, extension string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "app", "service"+extension)
	local := filepath.Join(root, "include", "detail.h")
	system := filepath.Join(root, "app", "system.h")
	paths := writeCFamilyNavigationFiles(t, map[string]string{
		source: `#include "../include/detail.h"
#include <system.h>
int run(void) { return 0; }
`,
		local:  "int detail(void);\n",
		system: "int system_value(void);\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	if len(graph.Imports) != 2 {
		t.Fatalf("imports=%+v", graph.Imports)
	}
	for _, item := range graph.Imports {
		switch item.ImportPath {
		case "../include/detail.h":
			if len(item.TargetPaths) != 1 || item.TargetPaths[0] != local {
				t.Fatalf("quoted include=%+v", item)
			}
		case "system.h":
			if len(item.TargetPaths) != 0 {
				t.Fatalf("system include should remain unresolved: %+v", item)
			}
		}
	}
}

func TestCFamilyQuotedIncludesDoNotSearchByBasename(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app", "service.c")
	other := filepath.Join(root, "other", "detail.h")
	paths := writeCFamilyNavigationFiles(t, map[string]string{
		source: "#include \"detail.h\"\n",
		other:  "int detail(void);\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	if len(graph.Imports) != 1 || len(graph.Imports[0].TargetPaths) != 0 {
		t.Fatalf("imports=%+v", graph.Imports)
	}
}

func writeCFamilyNavigationFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}
