package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTypeScriptNavigationImportsResolveRelativeTargetPaths(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "web", "component.ts")
	caller := filepath.Join(root, "app", "use.ts")
	for path, content := range map[string]string{
		target: "export class Component {}\n",
		caller: "import { Component } from '../web/component'\nexport function use(value: Component) { return value }\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	graph, stats := BuildNavigationGraphWithOptions([]string{caller, target}, NavigationBuildOptions{DisableCache: true})
	if stats.Parsed != 2 || len(graph.Imports) != 1 {
		t.Fatalf("stats=%+v imports=%#v", stats, graph.Imports)
	}
	item := graph.Imports[0]
	if item.Alias != "Component" || item.ImportPath != "../web/component" || len(item.TargetPaths) != 1 || item.TargetPaths[0] != target {
		t.Fatalf("resolved import=%#v", item)
	}
}
