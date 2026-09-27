package analysis

import "testing"

func TestRustImportsReachDirectoryArchitecture(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "src/lib.rs", Content: []byte("mod feature;\nuse crate::feature::run;\npub fn boot() { run(); }\n")},
		{Path: "src/feature/mod.rs", Content: []byte("pub fn run() {}\n")},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	report := BuildArchitecture(universe)
	coverage := report.RelationCoverage
	if coverage.ImportFacts != 2 || coverage.ResolvedImports != 2 || coverage.UnresolvedImports != 0 || coverage.AmbiguousImports != 0 {
		t.Fatalf("coverage=%+v", coverage)
	}
	for _, relation := range report.Relations {
		if relation.From == "src" && relation.To == "src/feature" && relation.Kind == "import" && relation.Count == 2 {
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}

func TestRustInlineAndExplicitPathModulesReachDirectoryArchitecture(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "src/lib.rs", Content: []byte("#[path = r#\"platform/unix.rs\"#]\nmod platform;\nmod local { pub fn run() {} }\npub fn boot() { platform::run(); local::run(); }\n")},
		{Path: "src/platform/unix.rs", Content: []byte("pub fn run() {}\n")},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	report := BuildArchitecture(universe)
	coverage := report.RelationCoverage
	if coverage.ImportFacts != 2 || coverage.ResolvedImports != 2 || coverage.UnresolvedImports != 0 || coverage.AmbiguousImports != 0 {
		t.Fatalf("coverage=%+v", coverage)
	}
	for _, relation := range report.Relations {
		if relation.From == "src" && relation.To == "src/platform" && relation.Kind == "import" && relation.Count == 1 {
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}
