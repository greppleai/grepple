package analysis

import "testing"

func TestImportedParameterTypesReachDirectoryArchitecture(t *testing.T) {
	tests := []struct {
		name, target string
		sources      []Source
	}{
		{
			name: "python", target: "pkg",
			sources: []Source{
				{Path: "app/service.py", Content: []byte("from pkg.models import Model\ndef use(value: Model):\n    pass\n")},
				{Path: "pkg/models.py", Content: []byte("class Model:\n    pass\n")},
			},
		},
		{
			name: "java", target: "lib/api",
			sources: []Source{
				{Path: "app/Service.java", Content: []byte("package app;\nimport lib.api.Helper;\nclass Service { void use(Helper value) {} }\n")},
				{Path: "lib/api/Helper.java", Content: []byte("package lib.api;\npublic class Helper {}\n")},
			},
		},
		{
			name: "kotlin", target: "lib/api",
			sources: []Source{
				{Path: "app/Service.kt", Content: []byte("package app\nimport lib.api.Helper\nfun use(value: Helper) {}\n")},
				{Path: "lib/api/Helper.kt", Content: []byte("package lib.api\nclass Helper\n")},
			},
		},
		{
			name: "rust", target: "src/model",
			sources: []Source{
				{Path: "src/lib.rs", Content: []byte("mod model;\nuse crate::model::Thing;\npub fn use_it(value: Thing) {}\n")},
				{Path: "src/model/mod.rs", Content: []byte("pub struct Thing;\n")},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertImportedTypeRelation(t, test.sources, test.target, "parameter")
		})
	}
}

func assertImportedTypeRelation(t *testing.T, sources []Source, target, role string) {
	t.Helper()
	universe, err := NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	report := BuildArchitecture(universe)
	if report.RelationCoverage.ResolvedTypeReferences == 0 {
		t.Fatalf("coverage=%+v relations=%+v", report.RelationCoverage, report.Relations)
	}
	for _, relation := range report.Relations {
		fromSource := relation.From == "app" || relation.From == "src"
		hasRole := len(relation.Evidence) > 0 && relation.Evidence[0].Role == role
		if fromSource && relation.To == target && relation.Kind == "type-reference" && hasRole {
			return
		}
	}
	t.Fatalf("relations=%+v", report.Relations)
}

func TestImportedFieldTypesReachDirectoryArchitecture(t *testing.T) {
	tests := []struct {
		name, target string
		sources      []Source
	}{
		{
			name: "python", target: "pkg",
			sources: []Source{
				{Path: "app/service.py", Content: []byte("from pkg.models import Model\nclass Service:\n    item: Model\n")},
				{Path: "pkg/models.py", Content: []byte("class Model:\n    pass\n")},
			},
		},
		{
			name: "kotlin", target: "lib/api",
			sources: []Source{
				{Path: "app/Service.kt", Content: []byte("package app\nimport lib.api.Helper\nclass Service(val item: Helper)\n")},
				{Path: "lib/api/Helper.kt", Content: []byte("package lib.api\nclass Helper\n")},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertImportedTypeRelation(t, test.sources, test.target, "field")
		})
	}
}

func TestCSharpAliasParameterTypeRemainsUnqualified(t *testing.T) {
	universe, err := NewUniverse([]Source{
		{Path: "app/Service.cs", Content: []byte("using HelperAlias = Lib.Api.Helper;\nnamespace App;\npublic class Service { HelperAlias Item; void Use(HelperAlias value) {} }\n")},
		{Path: "lib/api/Helper.cs", Content: []byte("namespace Lib.Api;\npublic class Helper {}\n")},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	report := BuildArchitecture(universe)
	coverage := report.RelationCoverage
	if coverage.TypeReferences != 2 || coverage.UnqualifiedTypeReferences != 2 || coverage.ResolvedTypeReferences != 0 {
		t.Fatalf("coverage=%+v relations=%+v", coverage, report.Relations)
	}
	for _, relation := range report.Relations {
		if relation.Kind == "type-reference" {
			t.Fatalf("C# alias was promoted into a type relation: %+v", relation)
		}
	}
}
