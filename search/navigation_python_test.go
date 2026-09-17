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
