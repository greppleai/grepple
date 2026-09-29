package parser

import "testing"

func TestLanguageForMinimumSupportedSet(t *testing.T) {
	tests := map[string]string{
		"main.go": "go", "app.js": "javascript", "app.ts": "typescript", "app.mts": "typescript",
		"app.cts": "typescript", "app.tsx": "tsx", "app.py": "python", "types.pyi": "python",
		"Main.java": "java", "Main.kt": "kotlin", "main.dart": "dart", "main.swift": "swift",
		"Program.cs": "csharp", "main.c": "c", "header.h": "c", "main.cpp": "cpp",
		"header.hpp": "cpp", "main.rs": "rust", "build.sh": "shell", "build.zsh": "shell",
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			if got := NewParser().LanguageFor(path); got != want {
				t.Fatalf("LanguageFor(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

func TestSupportedLanguagesOwnsClassificationMetadata(t *testing.T) {
	languages := NewParser().SupportedLanguages()
	if len(languages) != len(languageAdapters) {
		t.Fatalf("capabilities=%d adapters=%d", len(languages), len(languageAdapters))
	}
	for _, language := range languages {
		if _, ok := languageAdapters[language.ID]; !ok {
			t.Fatalf("capability %q has no parser adapter", language.ID)
		}
		if language.GrammarABI == 0 || len(language.GrammarFingerprint) != len("sha256:")+64 {
			t.Fatalf("capability %q has incomplete grammar identity: %#v", language.ID, language)
		}
		for _, extension := range language.Extensions {
			if got := NewParser().LanguageFor("source" + extension); got != language.ID {
				t.Fatalf("LanguageFor(%q)=%q, want %q", extension, got, language.ID)
			}
		}
	}
	languages[0].Extensions[0] = ".changed"
	fresh, _ := NewParser().CapabilitiesForLanguage("go")
	if len(fresh.Extensions) != 1 || fresh.Extensions[0] != ".go" || !fresh.ImportNavigation {
		t.Fatalf("Go capability metadata=%#v", fresh)
	}
	python, _ := NewParser().CapabilitiesForLanguage("python")
	if !python.ImportNavigation {
		t.Fatalf("Python import navigation missing: %#v", python)
	}
	assertImportNavigationCapabilities(t, "java", "kotlin", "csharp", "rust")
	assertEntrypointNavigationCapabilities(t, "go", "c", "cpp", "java", "javascript", "kotlin", "csharp", "python", "rust", "tsx", "typescript")
}

func assertImportNavigationCapabilities(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		language, _ := NewParser().CapabilitiesForLanguage(id)
		if !language.ImportNavigation {
			t.Fatalf("%s import navigation missing: %#v", id, language)
		}
	}
}

func assertEntrypointNavigationCapabilities(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		language, _ := NewParser().CapabilitiesForLanguage(id)
		if !language.EntrypointNavigation {
			t.Fatalf("%s entrypoint navigation missing: %#v", id, language)
		}
	}
}

func TestSupportedContentLanguagesIncludesSpecializedFormats(t *testing.T) {
	languages := NewParser().SupportedContentLanguages()
	byID := make(map[string]ContentLanguageCapabilities, len(languages))
	for _, language := range languages {
		if _, exists := byID[language.ID]; exists {
			t.Fatalf("duplicate content language %q", language.ID)
		}
		byID[language.ID] = language
	}
	for _, id := range []string{"go", "javascript", "markdown", "json", "yaml", "text"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing content language %q", id)
		}
	}
	if markdown := byID["markdown"]; !markdown.StructuralGrep || !markdown.Outline || !markdown.Specialized {
		t.Fatalf("markdown capabilities=%#v", markdown)
	}
	if json := byID["json"]; json.StructuralGrep || !json.Outline || !json.Specialized {
		t.Fatalf("json capabilities=%#v", json)
	}
	if goLanguage := byID["go"]; !goLanguage.StructuralGrep || !goLanguage.Outline || !goLanguage.Navigation || goLanguage.Specialized {
		t.Fatalf("go capabilities=%#v", goLanguage)
	}
}

func TestNavigationFactCapabilitiesAreAdapterOwned(t *testing.T) {
	want := map[string]NavigationFactCapabilities{
		"go":         {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"javascript": {Declarations: true, Calls: true, Imports: true, TypeReferences: true, MemberAccess: true, Entrypoints: true},
		"typescript": {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"tsx":        {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"python":     {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"java":       {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"kotlin":     {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"dart":       {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"swift":      {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true},
		"csharp":     {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"c":          {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"cpp":        {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"rust":       {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true, Entrypoints: true},
		"php":        {Declarations: true, Calls: true, Imports: true, TypeReferences: true, Fields: true, MemberAccess: true},
		"shell":      {Declarations: true, Calls: true},
	}
	for _, language := range NewParser().SupportedLanguages() {
		if language.NavigationFacts != want[language.ID] {
			t.Fatalf("%s facts=%#v, want %#v", language.ID, language.NavigationFacts, want[language.ID])
		}
	}
}

func TestAdvertisedTypedFieldAndMemberFactsHaveRepresentativeEvidence(t *testing.T) {
	tests := []struct {
		language, content               string
		typeReferences, fields, members bool
	}{
		{"go", "package p\ntype Foo struct{ State int }\nfunc use(value Foo){ _ = value.State }\n", true, true, true},
		{"javascript", "class Foo { state = 0; use(value) { return value.state; } }\n", true, false, true},
		{"typescript", "class Foo { state: number = 0; use(value: Foo){ return value.state; } }\n", true, true, true},
		{"tsx", "class Foo { state: number = 0; use(value: Foo){ return value.state; } }\n", true, true, true},
		{"python", "class Foo:\n    state: int = 0\ndef use(value: Foo):\n    return value.state\n", true, true, true},
		{"java", "class Foo { int state; int use(Foo value){ return value.state; } }\n", true, true, true},
		{"kotlin", "class Foo(var state: Int)\nfun use(value: Foo): Int { return value.state }\n", true, true, true},
		{"csharp", "class Foo { public int State; int Use(Foo value){ return value.State; } }\n", true, true, true},
		{"c", "struct Foo { int state; };\nint use(struct Foo value){ return value.state; }\n", true, true, true},
		{"cpp", "struct Foo { int state; };\nint use(Foo value){ return value.state; }\n", true, true, true},
		{"rust", "struct Foo { state: i32 }\nfn use(value: Foo) -> i32 { value.state }\n", true, true, true},
		{"php", "<?php class Foo { public int $state; function use(Foo $value): int { return $value->state; } }\n", true, true, true},
		{"swift", "struct Foo { let state: Int\nfunc use(value: Foo) -> Int { return value.state } }\n", true, true, false},
		{"shell", "use() { echo value; }\n", false, false, false},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, "sample")
			if got := len(graph.TypeUsages) > 0; got != test.typeReferences {
				t.Fatalf("type references=%v facts=%#v", got, graph.TypeUsages)
			}
			if got := len(graph.Fields) > 0; got != test.fields {
				t.Fatalf("fields=%v facts=%#v", got, graph.Fields)
			}
			if got := len(graph.MemberAccesses) > 0; got != test.members {
				t.Fatalf("member accesses=%v facts=%#v", got, graph.MemberAccesses)
			}
		})
	}
}

func TestNavigationFieldsRemainClassOwned(t *testing.T) {
	tests := []struct {
		language, content string
		want              map[string]string
	}{
		{
			language: "python",
			content:  "class Foo:\n    state: int\n    def method(self):\n        local: str\n",
			want:     map[string]string{"state": "int"},
		},
		{
			language: "kotlin",
			content:  "class Foo(input: String, val state: Int) {\n  var other: String = \"\"\n  fun method() { val local: Long = 0 }\n}\n",
			want:     map[string]string{"state": "Int", "other": "String"},
		},
		{
			language: "csharp",
			content:  "class Foo { int first, second; string Property { get; set; } void Method() { int local; } }\n",
			want:     map[string]string{"first": "int", "second": "int", "Property": "string"},
		},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, "sample")
			got := make(map[string]string, len(graph.Fields))
			for _, field := range graph.Fields {
				got[field.Name] = field.Type
			}
			if len(got) != len(test.want) {
				t.Fatalf("fields=%#v", graph.Fields)
			}
			for name, fieldType := range test.want {
				if got[name] != fieldType {
					t.Fatalf("field %q=%q, want %q; facts=%#v", name, got[name], fieldType, graph.Fields)
				}
			}
		})
	}
}

func TestNestedNavigationFieldsRemainOnNearestOwner(t *testing.T) {
	graph := BuildNavigationGraph("class Outer { int own; class Inner { string nested; } }\n", "csharp", "sample.cs")
	owners := map[string]map[string]bool{}
	for _, field := range graph.Fields {
		if owners[field.OwnerType] == nil {
			owners[field.OwnerType] = map[string]bool{}
		}
		owners[field.OwnerType][field.Name] = true
	}
	if !owners["Outer"]["own"] || owners["Outer"]["nested"] || !owners["Inner"]["nested"] {
		t.Fatalf("fields=%#v", graph.Fields)
	}
}

func TestCAnonymousTypedefDoesNotInventNavigationFieldOwner(t *testing.T) {
	graph := BuildNavigationGraph("typedef struct { int state; } Foo;\n", "c", "sample.c")
	if len(graph.Fields) != 0 {
		t.Fatalf("anonymous typedef fields=%#v", graph.Fields)
	}
}

func TestGrammarCardinalityComesFromGeneratedMetadata(t *testing.T) {
	tests := []struct {
		language, parent, field string
		want                    GrammarCardinality
	}{
		{language: "go", parent: "parameter_declaration", field: "name", want: GrammarCardinalityMany},
		{language: "go", parent: "function_declaration", field: "name", want: GrammarCardinalityOne},
		{language: "go", parent: "argument_list", want: GrammarCardinalityMany},
		{language: "typescript", parent: "arguments", want: GrammarCardinalityMany},
		{language: "tsx", parent: "jsx_element", field: "open_tag", want: GrammarCardinalityOne},
	}
	for _, test := range tests {
		grammar := NewParser().GetGrammar(test.language)
		got := grammar.ChildrenCardinality(test.parent)
		if test.field != "" {
			got = grammar.FieldCardinality(test.parent, test.field)
		}
		if got != test.want {
			t.Fatalf("cardinality %s/%s/%s=%d, want %d", test.language, test.parent, test.field, got, test.want)
		}
	}
	if got := NewParser().GetGrammar("unknown").FieldCardinality("node", "field"); got != GrammarCardinalityUnknown {
		t.Fatalf("unknown cardinality=%d", got)
	}
}
func TestGrammarSubtypeComesFromGeneratedMetadata(t *testing.T) {
	tests := []struct {
		language, supertype, kind string
		want                      bool
	}{
		{language: "go", supertype: "_expression", kind: "identifier", want: true},
		{language: "go", supertype: "_statement", kind: "assignment_statement", want: true},
		{language: "go", supertype: "_type", kind: "pointer_type", want: true},
		{language: "typescript", supertype: "expression", kind: "call_expression", want: true},
		{language: "typescript", supertype: "expression", kind: "identifier", want: true},
		{language: "tsx", supertype: "expression", kind: "jsx_element", want: true},
		{language: "typescript", supertype: "declaration", kind: "interface_declaration", want: true},
		{language: "typescript", supertype: "expression", kind: "interface_declaration", want: false},
	}
	for _, test := range tests {
		if got := NewParser().GetGrammar(test.language).Subtype(test.supertype, test.kind); got != test.want {
			t.Fatalf("GrammarSubtype(%q, %q, %q)=%v, want %v", test.language, test.supertype, test.kind, got, test.want)
		}
	}
}

func TestNewLanguageOutlines(t *testing.T) {
	tests := []struct {
		path    string
		content string
		kind    string
		name    string
	}{
		{path: "app.dart", content: "class Service { int run() { return 1; } }\n", kind: "method", name: "run"},
		{path: "app.py", content: "class Service:\n    def run(self):\n        return 1\n", kind: "function", name: "run"},
		{path: "App.cs", content: "namespace App { public class Service { public int Run() { return 1; } } }", kind: "method", name: "Run"},
		{path: "app.c", content: "typedef struct Item { int value; } Item;\nint run(int x) { return x; }\n", kind: "function", name: "run"},
		{path: "app.cpp", content: "namespace app { class Service { public: int run() { return 1; } }; }\n", kind: "method", name: "run"},
		{path: "app.rs", content: "trait Runner { fn run(&self); }\nimpl Runner for Service { fn run(&self) {} }\n", kind: "method", name: "run"},
		{path: "build.sh", content: "build() { echo build; }\n", kind: "function", name: "build"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			outline := OutlineFile(test.path, test.content)
			mustFind(t, outline.Symbols, test.kind, test.name)
		})
	}
}

func TestNewLanguagesBuildStructuralSegments(t *testing.T) {
	tests := []struct {
		language string
		content  string
		hitLine  int
	}{
		{language: "dart", content: "void run() {\n print(1);\n}\n", hitLine: 2},
		{language: "python", content: "def run():\n    return 1\n", hitLine: 2},
		{language: "csharp", content: "class App { int Run() {\n return 1;\n} }\n", hitLine: 2},
		{language: "c", content: "int run() {\n return 1;\n}\n", hitLine: 2},
		{language: "cpp", content: "int run() {\n return 1;\n}\n", hitLine: 2},
		{language: "rust", content: "fn run() {\n  return;\n}\n", hitLine: 2},
		{language: "shell", content: "run() {\n  echo yes\n}\n", hitLine: 2},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			segments := BuildSegments(test.content, test.language, map[int]bool{test.hitLine: true})
			if len(segments) == 0 {
				t.Fatal("expected structural segments")
			}
			if len(segments) == 1 && segments[0].Start == test.hitLine && segments[0].End == test.hitLine {
				t.Fatalf("expected structural context around hit line, got %#v", segments)
			}
		})
	}
}
