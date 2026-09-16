package analysis

import "testing"

func TestCSharpImportsReachDirectoryArchitecture(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "app/Service.cs", Content: []byte("using Lib.Api;\nnamespace App;\npublic class Service { void Use() { Helper.Work(); } }\n")},
		{Path: "lib/api/Helper.cs", Content: []byte("namespace Lib.Api;\npublic class Helper { public static void Work() {} }\n")},
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
		if relation.From == "app" && relation.To == "lib/api" && relation.Kind == "import" && len(relation.Evidence) == 1 && relation.Evidence[0].Confidence == "local-import-resolved" {
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}
