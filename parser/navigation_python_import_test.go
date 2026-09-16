package parser

import "testing"

func TestPythonNavigationImportsPreserveAliasesAndCallContext(t *testing.T) {
	content := `import package.module
import package.other as other
from package.models import User, load as fetch
from .local import run
from ..shared import *

def use():
    import hidden.module as hidden
    fetch()
    other.start()
    run()
`
	graph := BuildNavigationGraph(content, "python", "app/service.py")
	imports := make(map[string]NavigationImport, len(graph.Imports))
	for _, item := range graph.Imports {
		imports[item.Alias] = item
	}
	if len(imports) != 6 {
		t.Fatalf("module-level imports=%#v", graph.Imports)
	}
	if _, ok := imports["hidden"]; ok {
		t.Fatalf("function-local import was promoted to module scope: %#v", graph.Imports)
	}
	for alias, expected := range map[string]struct {
		path, imported string
		line           int
	}{
		"package": {"package.module", "*", 1},
		"other":   {"package.other", "*", 2},
		"User":    {"package.models", "User", 3},
		"fetch":   {"package.models", "load", 3},
		"run":     {".local", "run", 4},
		"*":       {"..shared", "*", 5},
	} {
		item, ok := imports[alias]
		if !ok || item.ImportPath != expected.path || item.Imported != expected.imported || item.Line != expected.line {
			t.Fatalf("import %q=%#v all=%#v", alias, item, graph.Imports)
		}
	}
	calls := make(map[string]NavigationCall, len(graph.Calls))
	for _, call := range graph.Calls {
		calls[call.Display] = call
	}
	if call := calls["fetch"]; call.ImportPath != "package.models" || call.ResolvedName != "load" {
		t.Fatalf("fetch call=%#v", call)
	}
	if call := calls["other.start"]; call.ImportPath != "package.other" || call.ResolvedName != "start" {
		t.Fatalf("qualified call=%#v", call)
	}
	if call := calls["run"]; call.ImportPath != ".local" || call.ResolvedName != "run" {
		t.Fatalf("relative call=%#v", call)
	}
}

func TestPythonNavigationDuplicateAliasesRemainFactsButNotBindings(t *testing.T) {
	content := "import first.module as duplicate\nimport second.module as duplicate\ndef use():\n    duplicate.run()\n"
	graph := BuildNavigationGraph(content, "python", "service.py")
	if len(graph.Imports) != 2 || graph.Imports[0].Alias != "duplicate" || graph.Imports[1].Alias != "duplicate" || graph.Imports[0].ImportPath == graph.Imports[1].ImportPath {
		t.Fatalf("imports=%#v", graph.Imports)
	}
	if len(graph.Calls) != 1 || graph.Calls[0].ImportPath != "" || graph.Calls[0].Confidence != "" {
		t.Fatalf("ambiguous alias call=%#v", graph.Calls)
	}
}

func TestCachedPythonNavigationImportsInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "from package.models import load as fetch\ndef use():\n    fetch()\n"
	cold, _, coldHit, err := CachedNavigationGraph(content, "python", "first/service.py")
	if err != nil {
		t.Fatal(err)
	}
	warm, _, warmHit, err := CachedNavigationGraph(content, "python", "moved/service.py")
	if err != nil {
		t.Fatal(err)
	}
	if coldHit || !warmHit || len(cold.Imports) != 1 || len(warm.Imports) != 1 {
		t.Fatalf("coldHit=%v warmHit=%v cold=%#v warm=%#v", coldHit, warmHit, cold.Imports, warm.Imports)
	}
	if cold.Imports[0].Path != "first/service.py" || warm.Imports[0].Path != "moved/service.py" || cold.Imports[0].Alias != warm.Imports[0].Alias || cold.Imports[0].ImportPath != warm.Imports[0].ImportPath {
		t.Fatalf("cold=%#v warm=%#v", cold.Imports[0], warm.Imports[0])
	}
}
