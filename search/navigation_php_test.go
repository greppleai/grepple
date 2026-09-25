package search

import (
	"path/filepath"
	"testing"
)

func TestPHPImportedClassResolvesWithoutComposerGuessing(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "Service.php")
	helper := filepath.Join(root, "lib", "Formatter.php")
	decoy := filepath.Join(root, "other", "Formatter.php")
	paths := writeJVMNavigationFiles(t, map[string]string{
		service: "<?php\nnamespace App;\nuse Lib\\Formatter as Format;\nfunction run() { Format::render(); }\n",
		helper:  "<?php\nnamespace Lib;\nclass Formatter { public static function render(): void {} }\n",
		decoy:   "<?php\nnamespace Other;\nclass Formatter { public static function render(): void {} }\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertJVMImportTarget(t, graph.Imports, "Format", helper)
	for _, call := range graph.Calls {
		if call.Path == service && call.Name == "render" {
			if call.TargetID == "" || call.Confidence != "import-resolved" {
				t.Fatalf("PHP call not import-resolved: %#v", call)
			}
			for _, decl := range graph.Declarations {
				if decl.ID == call.TargetID && decl.Path == helper {
					return
				}
			}
			t.Fatalf("PHP call resolved to wrong declaration: %#v", call)
		}
	}
	t.Fatalf("PHP call missing: %#v", graph.Calls)
}
