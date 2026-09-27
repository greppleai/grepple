package navigation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPHPNamespaceUseResolvesAcrossFiles(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "service.php")
	model := filepath.Join(root, "model.php")
	for path, source := range map[string]string{
		service: "<?php namespace App\\Service; use App\\Model\\User; function run(): void { User::create(); }\n",
		model:   "<?php namespace App\\Model; class User { public static function create(): void {} }\n",
	} {
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	graph, _ := BuildGraphWithOptions([]string{service, model}, BuildOptions{DisableCache: true})
	if len(graph.Imports) != 1 || len(graph.Imports[0].TargetPaths) != 1 || graph.Imports[0].TargetPaths[0] != model {
		t.Fatalf("PHP import targets=%#v exports=%#v", graph.Imports, graph.Exports)
	}
	for _, call := range graph.Calls {
		if call.Name == "create" && call.TargetID != "" {
			return
		}
	}
	t.Fatalf("PHP cross-file call unresolved: %#v", graph.Calls)
}
