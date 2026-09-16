package analysis

import "testing"

func TestJavaImportsReachDirectoryArchitecture(t *testing.T) {
	report := buildJVMImportArchitecture(t, []Source{
		{Path: "app/Service.java", Content: []byte("package app;\nimport lib.api.Helper;\nclass Service { void use() { Helper.work(); } }\n")},
		{Path: "lib/api/Helper.java", Content: []byte("package lib.api;\npublic class Helper { public static void work() {} }\n")},
	})
	assertJVMImportArchitecture(t, report, "lib/api")
}

func TestKotlinImportsReachDirectoryArchitecture(t *testing.T) {
	report := buildJVMImportArchitecture(t, []Source{
		{Path: "app/Service.kt", Content: []byte("package app\nimport lib.tools.run\nclass Service { fun use() { run() } }\n")},
		{Path: "lib/tools/Actions.kt", Content: []byte("package lib.tools\nfun run() {}\n")},
	})
	assertJVMImportArchitecture(t, report, "lib/tools")
}

func buildJVMImportArchitecture(t *testing.T, sources []Source) ArchitectureReport {
	t.Helper()
	universe, err := NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(universe.Close)
	return BuildArchitecture(universe)
}

func assertJVMImportArchitecture(t *testing.T, report ArchitectureReport, target string) {
	t.Helper()
	coverage := report.RelationCoverage
	if coverage.ImportFacts != 1 || coverage.ResolvedImports != 1 || coverage.UnresolvedImports != 0 || coverage.AmbiguousImports != 0 {
		t.Fatalf("coverage=%+v", coverage)
	}
	for _, relation := range report.Relations {
		if relation.From == "app" && relation.To == target && relation.Kind == "import" && len(relation.Evidence) == 1 && relation.Evidence[0].Confidence == "local-import-resolved" {
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}
