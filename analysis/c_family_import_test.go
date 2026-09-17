package analysis

import "testing"

func TestCFamilyQuotedIncludesReachDirectoryArchitecture(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "app/service.c", Content: []byte("#include \"../include/detail.h\"\nint run(void) { return detail(); }\n")},
		{Path: "include/detail.h", Content: []byte("int detail(void);\n")},
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
		if relation.From == "app" && relation.To == "include" && relation.Kind == "import" && len(relation.Evidence) == 1 {
			evidence := relation.Evidence[0]
			if evidence.Path != "app/service.c" || evidence.Line != 1 || evidence.Caller != "*" || evidence.ImportPath != "../include/detail.h" || evidence.Confidence != "local-import-resolved" {
				t.Fatalf("evidence=%+v", evidence)
			}
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}
