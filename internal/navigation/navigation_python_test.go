package navigation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestPythonNavigationImportsResolveAbsoluteAndRelativeModules(t *testing.T) {
	root := t.TempDir()
	caller := filepath.Join(root, "app", "service.py")
	models := filepath.Join(root, "pkg", "models.py")
	helpers := filepath.Join(root, "pkg", "helpers", "__init__.py")
	local := filepath.Join(root, "app", "local.py")
	shared := filepath.Join(root, "shared", "tools.pyi")
	files := map[string]string{
		caller:  "from pkg.models import load as fetch\nimport pkg.helpers as helpers\nfrom .local import run\nfrom ..shared.tools import execute\ndef use():\n    fetch()\n    helpers.start()\n    run()\n    execute()\n",
		models:  "def load():\n    pass\n",
		helpers: "def start():\n    pass\n",
		local:   "def run():\n    pass\n",
		shared:  "def execute(): ...\n",
	}
	paths := writePythonNavigationFiles(t, files)
	graph, stats := BuildGraphWithOptions(paths, BuildOptions{DisableCache: true})
	if stats.Parsed != len(files) || len(graph.Imports) != 4 {
		t.Fatalf("stats=%+v imports=%#v", stats, graph.Imports)
	}
	assertPythonImportTargets(t, graph, map[string]string{"fetch": models, "helpers": helpers, "run": local, "execute": shared})
	assertPythonImportCalls(t, graph, []string{"fetch", "helpers.start", "run", "execute"})
}

func writePythonNavigationFiles(t *testing.T, files map[string]string) []string {
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

func assertPythonImportTargets(t *testing.T, graph parser.NavigationGraph, expected map[string]string) {
	t.Helper()
	imports := make(map[string][]string, len(graph.Imports))
	for _, item := range graph.Imports {
		imports[item.Alias] = item.TargetPaths
	}
	for alias, target := range expected {
		paths := imports[alias]
		if len(paths) != 1 || paths[0] != target {
			t.Fatalf("import %q targets=%#v, want %q; imports=%#v", alias, paths, target, graph.Imports)
		}
	}
}

func assertPythonImportCalls(t *testing.T, graph parser.NavigationGraph, expected []string) {
	t.Helper()
	calls := make(map[string]string)
	for _, call := range graph.Calls {
		if call.TargetID != "" {
			calls[call.Display] = call.Confidence
		}
	}
	for _, display := range expected {
		if calls[display] != "import-resolved" {
			t.Fatalf("call %q confidence=%q calls=%#v", display, calls[display], graph.Calls)
		}
	}
}

func TestPythonImportTargetsPreserveAmbiguousSourceRoots(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "pkg", "models.py")
	second := filepath.Join(root, "two", "pkg", "models.py")
	files := []string{first, second}
	targets := pythonImportTargetFiles(files, filepath.Join(root, "two", "app.py"), "pkg.models")
	if len(targets) != 2 || targets[0] != first || targets[1] != second {
		t.Fatalf("targets=%#v", targets)
	}
	unrelated := filepath.Join(root, "vendor", "pkg", "models.py")
	if targets := pythonImportTargetFiles([]string{unrelated}, filepath.Join(root, "app.py"), "pkg.models"); len(targets) != 0 {
		t.Fatalf("unrelated nested source root resolved: %#v", targets)
	}
}
