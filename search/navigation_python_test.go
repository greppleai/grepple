package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPythonImportedParameterTypeResolvesReceiverCall(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "service.py")
	model := filepath.Join(root, "pkg", "models.py")
	paths := writePythonNavigationFiles(t, map[string]string{
		service: "from pkg.models import Model\ndef use(value: Model):\n    value.load()\n",
		model:   "class Model:\n    def load(self):\n        pass\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	for _, call := range graph.Calls {
		strong := call.Confidence == "exact" || call.Confidence == "import-resolved" || call.Confidence == "context-resolved"
		if call.Path == service && call.Display == "value.load" && call.TargetID != "" && strong {
			return
		}
	}
	t.Fatalf("missing resolved typed receiver call: %#v", graph.Calls)
}

func TestPythonAmbiguousImportedParameterTypePreservesCallCandidates(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "service.py")
	first := filepath.Join(root, "one", "pkg", "models.py")
	second := filepath.Join(root, "two", "pkg", "models.py")
	paths := writePythonNavigationFiles(t, map[string]string{
		service: "from pkg.models import Model\ndef use(value: Model):\n    value.load()\n",
		first:   "class Model:\n    def load(self):\n        pass\n",
		second:  "class Model:\n    def load(self):\n        pass\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	for _, call := range graph.Calls {
		if call.Path == service && call.Display == "value.load" && call.TargetID == "" && len(call.CandidateTargetIDs) == 2 && call.Confidence == "candidate" {
			return
		}
	}
	t.Fatalf("ambiguous imported receiver call lost candidates: imports=%#v calls=%#v", graph.Imports, graph.Calls)
}

func TestPythonExternalImportedParameterTypeRemainsUnresolved(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "app", "service.py")
	paths := writePythonNavigationFiles(t, map[string]string{
		service: "from external.models import Model\ndef use(value: Model):\n    value.load()\n",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	unresolvedImport := false
	for _, item := range graph.Imports {
		if item.Path == service && item.Alias == "Model" && len(item.TargetPaths) == 0 {
			unresolvedImport = true
			break
		}
	}
	if !unresolvedImport {
		t.Fatalf("external import was not retained as unresolved evidence: %#v", graph.Imports)
	}
	for _, call := range graph.Calls {
		if call.Path == service && call.Display == "value.load" && call.TargetID == "" && len(call.CandidateTargetIDs) == 0 && call.Confidence == "candidate" {
			return
		}
	}
	t.Fatalf("external typed receiver call was not retained as unresolved evidence: %#v", graph.Calls)
}

func writePythonNavigationFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}
