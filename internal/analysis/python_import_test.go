package analysis

import "testing"

func TestPythonImportsReachDirectoryArchitecture(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "app/service.py", Content: []byte("from pkg.models import load as fetch\ndef use():\n    fetch()\n")},
		{Path: "pkg/models.py", Content: []byte("def load():\n    pass\n")},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	report := BuildArchitecture(universe)
	coverage := report.RelationCoverage
	if coverage.ImportFacts != 1 || coverage.ResolvedImports != 1 || coverage.UnresolvedImports != 0 || coverage.AmbiguousImports != 0 {
		t.Fatalf("coverage=%+v", coverage)
	}
	for _, relation := range report.Relations {
		if relation.From == "app" && relation.To == "pkg" && relation.Kind == "import" && len(relation.Evidence) == 1 {
			evidence := relation.Evidence[0]
			if evidence.Path != "app/service.py" || evidence.Line != 1 || evidence.Caller != "fetch" || evidence.ImportPath != "pkg.models" || evidence.Confidence != "local-import-resolved" {
				t.Fatalf("evidence=%+v", evidence)
			}
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}
