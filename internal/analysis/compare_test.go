package analysis

import "testing"

func TestCompareArchitecturesNormalizesPathsAndOrdering(t *testing.T) {
	before := architectureComparisonFixture()
	before.Root = `.\`
	before.SourceFiles[0].Path = `service\service.go`
	before.Symbols[0].Path = `service\service.go`
	before.Symbols[0].Directory = `service\.`
	before.Directories[1].Languages = []ArchitectureCount{{Name: "typescript", Count: 1}, {Name: "go", Count: 1}}
	after := architectureComparisonFixture()
	if difference := CompareArchitectures(before, after); difference != nil {
		t.Fatalf("difference=%+v", difference)
	}
}

func TestCompareArchitecturesReportsFirstSourceLinkedDifference(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*ArchitectureReport)
		kind     string
		identity string
		path     string
	}{
		{name: "file", mutate: func(value *ArchitectureReport) { value.SourceFiles[0].Classification = "test" }, kind: "file", identity: "service/service.go", path: "service/service.go"},
		{name: "declaration", mutate: func(value *ArchitectureReport) { value.Symbols[0].End++ }, kind: "declaration", identity: "service/service.go|go|func||Run", path: "service/service.go"},
		{name: "relation", mutate: func(value *ArchitectureReport) { value.Relations[0].Count++ }, kind: "relation", identity: "app|service|resolved-call", path: "app/main.go"},
		{name: "directory", mutate: func(value *ArchitectureReport) { value.Directories[1].PublicCallables++ }, kind: "directory", identity: "service", path: "service"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := architectureComparisonFixture()
			after := architectureComparisonFixture()
			test.mutate(&after)
			difference := CompareArchitectures(before, after)
			if difference == nil || difference.Kind != test.kind || difference.Change != "changed" || difference.Identity != test.identity || difference.Path != test.path {
				t.Fatalf("difference=%+v", difference)
			}
			if len(difference.Before) == 0 || len(difference.After) == 0 {
				t.Fatalf("difference lacks before/after values: %+v", difference)
			}
		})
	}
}

func architectureComparisonFixture() ArchitectureReport {
	return ArchitectureReport{
		Schema: ArchitectureSchema,
		Root:   ".",
		Files:  1,
		Sources: SourceSummary{
			Discovered: 1, Selected: 1, Parsed: 1,
		},
		SourceFiles: []ArchitectureSourceFile{{Path: "service/service.go", Language: "go", Classification: "production"}},
		Directories: []ArchitectureDirectory{
			{Path: ".", Files: 1, Classifications: []ArchitectureCount{{Name: "production", Count: 1}}, Languages: []ArchitectureCount{{Name: "go", Count: 1}}, Declarations: []ArchitectureCount{{Name: "func", Count: 1}}, PublicCallables: 1},
			{Path: "service", Files: 1, Classifications: []ArchitectureCount{{Name: "production", Count: 1}}, Languages: []ArchitectureCount{{Name: "go", Count: 1}, {Name: "typescript", Count: 1}}, Declarations: []ArchitectureCount{{Name: "func", Count: 1}}, PublicCallables: 1},
		},
		Symbols:         []ArchitectureSymbol{{Name: "Run", Kind: "func", Language: "go", Classification: "production", Path: "service/service.go", Directory: "service", Visibility: "public", Start: 3, End: 5}},
		Relations:       []ArchitectureRelation{{From: "app", To: "service", Kind: "resolved-call", Count: 1, Classifications: []ArchitectureCount{{Name: "production", Count: 1}}, Evidence: []ArchitectureRelationEvidence{{Path: "app/main.go", Line: 7, Caller: "main", Target: "Run", Kind: "resolved-call", Classification: "production", Confidence: "import-resolved"}}}},
		RepositoryRoots: []string{"example.com/project"},
	}
}
