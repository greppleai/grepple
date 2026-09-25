package navigation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPHPComposerPSR4RestrictsLocalImportTargets(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "composer.json")
	if err := os.WriteFile(manifest, []byte(`{"autoload":{"psr-4":{"App\\":["src/","lib/"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := filepath.Join(root, "app.php")
	correct := filepath.Join(root, "src", "Widget.php")
	alternate := filepath.Join(root, "lib", "Widget.php")
	wrong := filepath.Join(root, "other", "Widget.php")
	paths := writePythonNavigationFiles(t, map[string]string{
		caller: `<?php
use App\Widget;
function execute(): void { (new Widget())->run(); }
`,
		correct:   `<?php namespace App; class Widget { public function run(): void {} }`,
		alternate: `<?php namespace App; class Widget { public function run(): void {} }`,
		wrong:     `<?php namespace App; class Widget { public function run(): void {} }`,
	})
	graph, _ := BuildGraphWithOptions(paths, BuildOptions{DisableCache: true})
	var got []string
	for _, imp := range graph.Imports {
		if imp.Path == caller && imp.Alias == "Widget" {
			got = imp.TargetPaths
		}
	}
	if len(got) != 2 || got[0] != alternate && got[0] != correct || got[1] != alternate && got[1] != correct || got[0] == got[1] {
		t.Fatalf("PSR-4 import targets = %v; graph imports = %+v", got, graph.Imports)
	}
}

func TestPHPComposerPSR4RejectsWrongPathForKnownNamespace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"App\\":"src/"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	caller := filepath.Join(root, "caller.php")
	wrong := filepath.Join(root, "other", "Widget.php")
	paths := writePythonNavigationFiles(t, map[string]string{
		caller: `<?php use App\Widget; function execute(): void { (new Widget())->run(); }`,
		wrong:  `<?php namespace App; class Widget { public function run(): void {} }`,
	})
	graph, _ := BuildGraphWithOptions(paths, BuildOptions{DisableCache: true})
	for _, imp := range graph.Imports {
		if imp.Path == caller && imp.Alias == "Widget" && len(imp.TargetPaths) != 0 {
			t.Fatalf("wrong-path import = %+v", imp)
		}
	}
}
