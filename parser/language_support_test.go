package parser

import "testing"

func TestLanguageForMinimumSupportedSet(t *testing.T) {
	tests := map[string]string{
		"main.go": "go", "app.js": "javascript", "app.ts": "typescript", "app.mts": "typescript",
		"app.cts": "typescript", "app.tsx": "tsx", "app.py": "python", "types.pyi": "python",
		"Main.java": "java", "Main.kt": "kotlin",
		"Program.cs": "csharp", "main.c": "c", "header.h": "c", "main.cpp": "cpp",
		"header.hpp": "cpp", "main.rs": "rust", "build.sh": "shell", "build.zsh": "shell",
	}
	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			if got := LanguageFor(path); got != want {
				t.Fatalf("LanguageFor(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

func TestSupportedLanguagesOwnsClassificationMetadata(t *testing.T) {
	languages := SupportedLanguages()
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
			if got := LanguageFor("source" + extension); got != language.ID {
				t.Fatalf("LanguageFor(%q)=%q, want %q", extension, got, language.ID)
			}
		}
	}
	languages[0].Extensions[0] = ".changed"
	fresh, _ := CapabilitiesForLanguage("go")
	if len(fresh.Extensions) != 1 || fresh.Extensions[0] != ".go" || !fresh.ImportNavigation {
		t.Fatalf("Go capability metadata=%#v", fresh)
	}
	python, _ := CapabilitiesForLanguage("python")
	if !python.ImportNavigation {
		t.Fatalf("Python import navigation missing: %#v", python)
	}
	assertImportNavigationCapabilities(t, "java", "kotlin", "csharp")
}

func assertImportNavigationCapabilities(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		language, _ := CapabilitiesForLanguage(id)
		if !language.ImportNavigation {
			t.Fatalf("%s import navigation missing: %#v", id, language)
		}
	}
}

func TestSupportedContentLanguagesIncludesSpecializedFormats(t *testing.T) {
	languages := SupportedContentLanguages()
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
		got := GrammarChildrenCardinality(test.language, test.parent)
		if test.field != "" {
			got = GrammarFieldCardinality(test.language, test.parent, test.field)
		}
		if got != test.want {
			t.Fatalf("cardinality %s/%s/%s=%d, want %d", test.language, test.parent, test.field, got, test.want)
		}
	}
	if got := GrammarFieldCardinality("unknown", "node", "field"); got != GrammarCardinalityUnknown {
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
		if got := GrammarSubtype(test.language, test.supertype, test.kind); got != test.want {
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
		{language: "python", content: "def run():\n    return 1\n", hitLine: 2},
		{language: "csharp", content: "class App { int Run() {\n return 1;\n} }\n", hitLine: 2},
		{language: "c", content: "int run() {\n return 1;\n}\n", hitLine: 2},
		{language: "cpp", content: "int run() {\n return 1;\n}\n", hitLine: 2},
		{language: "rust", content: "fn run() {\n  return;\n}\n", hitLine: 2},
		{language: "shell", content: "run() {\n  echo yes\n}\n", hitLine: 2},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			segments := BuildSegments(test.content, test.language, map[int]bool{test.hitLine: true}, 20)
			if len(segments) == 0 {
				t.Fatal("expected structural segments")
			}
			if len(segments) == 1 && segments[0].Start == test.hitLine && segments[0].End == test.hitLine {
				t.Fatalf("expected structural context around hit line, got %#v", segments)
			}
		})
	}
}
