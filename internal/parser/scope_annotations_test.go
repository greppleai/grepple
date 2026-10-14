package parser

import (
	"reflect"
	"strings"
	"testing"
)

func TestCodeLineScopeAnnotationsAllRegisteredLanguages(t *testing.T) {
	tests := []struct {
		language, content string
		declaration, body int
		kind, scope       string
	}{
		{"go", "package p\nfunc run() {\n  work()\n}\n", 2, 3, "func", "func"},
		{"java", "class App {\n  void run() {\n    work();\n  }\n}\n", 2, 3, "method", "method"},
		{"kotlin", "fun run() {\n  println(1)\n}\n", 1, 2, "func", "func"},
		{"dart", "void run() {\n  print(1);\n}\n", 1, 2, "func", "func"},
		{"swift", "func run() {\n  print(1)\n}\n", 1, 2, "func", "func"},
		{"hcl", "resource \"test\" \"example\" {\n  name = \"one\"\n}\n", 1, 2, "block", "block"},
		{"svelte", "{#if ready}\n  <p>yes</p>\n{/if}\n", 1, 2, "if", ""},
		{"css", ".box {\n  color: red;\n}\n", 1, 2, "rule", "rule"},
		{"javascript", "function run() {\n  work();\n}\n", 1, 2, "func", "func"},
		{"typescript", "function run(): void {\n  work();\n}\n", 1, 2, "func", "func"},
		{"tsx", "function Run(): JSX.Element {\n  return <div />;\n}\n", 1, 2, "func", "func"},
		{"python", "def run():\n    work()\n", 1, 2, "func", "func"},
		{"csharp", "class App {\n  void Run() {\n    work();\n  }\n}\n", 2, 3, "method", "method"},
		{"c", "int run(void) {\n  return 1;\n}\n", 1, 2, "func", "func"},
		{"cpp", "int run() {\n  return 1;\n}\n", 1, 2, "func", "func"},
		{"rust", "fn run() {\n  work();\n}\n", 1, 2, "func", "func"},
		{"php", "<?php\nfunction run() {\n  work();\n}\n", 2, 3, "func", "func"},
		{"shell", "run() {\n  echo yes\n}\n", 1, 2, "func", "func"},
	}
	seen := make(map[string]bool)
	for _, tt := range tests {
		seen[tt.language] = true
		t.Run(tt.language, func(t *testing.T) {
			got := CodeLineScopeAnnotations(tt.content, tt.language, map[int]bool{tt.declaration: true, tt.body: true})
			for _, line := range []int{tt.declaration, tt.body} {
				a := got[line]
				wantKind, wantStart := tt.kind, tt.declaration
				if tt.language == "css" && line == tt.body {
					wantKind, wantStart = "property", tt.body
				}
				if a.Nearest == nil || a.Nearest.Kind != wantKind || a.Nearest.StartLine != wantStart || a.Nearest.EndLine < line {
					t.Errorf("line %d: nearest = %+v, want %s starting %d", line, a.Nearest, wantKind, wantStart)
				}
				if tt.scope == "" {
					if a.Scope != nil {
						t.Errorf("line %d: unexpected scope = %+v", line, a.Scope)
					}
				} else if a.Scope == nil || a.Scope.Kind != tt.scope || a.Scope.StartLine != tt.declaration || a.Scope.EndLine < line {
					t.Errorf("line %d: scope = %+v, want %s starting %d", line, a.Scope, tt.scope, tt.declaration)
				}
			}
		})
	}
	for _, language := range NewParser().SupportedContentLanguages() {
		if languageAdapters[language.ID] != nil && !seen[language.ID] {
			t.Errorf("missing registered code language %s", language.ID)
		}
	}
	if len(seen) != len(languageAdapters) {
		t.Errorf("fixtures for %d languages, registry has %d", len(seen), len(languageAdapters))
	}
}

func TestCodeLineScopeAnnotationsNestedAndNoGuesswork(t *testing.T) {
	content := "package p\n// outside\nfunc run() {\n  if ready {\n    work()\n  }\n  for true {\n    again()\n  }\n}\n"
	got := CodeLineScopeAnnotations(content, "go", map[int]bool{2: true, 3: true, 5: true, 8: true, 11: true})
	want := map[int]LineScopeAnnotation{
		3: {Nearest: &ScopeLineRange{"func", 3, 10}, Scope: &ScopeLineRange{"func", 3, 10}},
		5: {Nearest: &ScopeLineRange{"if", 4, 6}, Scope: &ScopeLineRange{"func", 3, 10}},
		8: {Nearest: &ScopeLineRange{"for", 7, 9}, Scope: &ScopeLineRange{"func", 3, 10}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if got := CodeLineScopeAnnotations("any @func:1-3 ⟦source-bytes:5⟧\n", "text", map[int]bool{1: true}); len(got) != 0 {
		t.Fatalf("text has no syntax: %#v", got)
	}
	if got := CodeLineScopeAnnotations(content, "unknown", map[int]bool{5: true}); len(got) != 0 {
		t.Fatalf("unsupported grammar: %#v", got)
	}
}

func TestCodeLineScopeAnnotationsMethodAndType(t *testing.T) {
	content := "class App {\n  run() {\n    if (ready) {\n      work();\n    }\n  }\n  field = 1;\n}\n"
	got := CodeLineScopeAnnotations(content, "javascript", map[int]bool{1: true, 4: true, 7: true})
	if a := got[4]; a.Nearest == nil || a.Nearest.Kind != "if" || a.Scope == nil || a.Scope.Kind != "method" || a.Scope.StartLine != 2 || a.Scope.EndLine != 6 {
		t.Fatalf("nested method: %+v", a)
	}
	if a := got[1]; a.Nearest == nil || a.Nearest.Kind != "class" || a.Scope == nil || a.Scope.Kind != "class" {
		t.Fatalf("class declaration: %+v", a)
	}
	if a := got[7]; a.Scope == nil || a.Scope.Kind != "class" {
		t.Fatalf("field scope: %+v", a)
	}
}

func TestCodeLineScopeAnnotationsAnonymousClassFieldIsNotMethod(t *testing.T) {
	content := "class App {\n  field = () => {\n    work();\n  };\n}\n"
	a := CodeLineScopeAnnotations(content, "javascript", map[int]bool{3: true})[3]
	if a.Scope == nil || a.Scope.Kind != "func" || a.Scope.StartLine != 2 {
		t.Fatalf("class field closure is a function, not a method: %+v", a)
	}
}

func TestCodeLineScopeAnnotationsEmbeddedSvelte(t *testing.T) {
	content := "<script lang=\"ts\">\nfunction run(): void {\n  if (ready) { work(); }\n}\n</script>\n<style>\n.box {\n  color: red;\n}\n</style>\n"
	got := CodeLineScopeAnnotations(content, "svelte", map[int]bool{3: true, 8: true})
	if a := got[3]; a.Scope == nil || a.Scope.Kind != "func" || a.Scope.StartLine != 2 || a.Nearest == nil || a.Nearest.Kind != "if" {
		t.Fatalf("injected TypeScript: %+v", a)
	}
	if a := got[8]; a.Scope == nil || a.Scope.Kind != "rule" || a.Scope.StartLine != 7 || a.Nearest == nil || a.Nearest.Kind != "property" {
		t.Fatalf("injected CSS: %+v", a)
	}
}

func TestCodeLineScopeAnnotationsRustMethodAndGoCase(t *testing.T) {
	rust := "struct App;\nimpl App {\n fn run(&self) {\n  if true {\n   work();\n  }\n }\n}\n"
	a := CodeLineScopeAnnotations(rust, "rust", map[int]bool{5: true})[5]
	if a.Nearest == nil || *a.Nearest != (ScopeLineRange{"if", 4, 6}) || a.Scope == nil || *a.Scope != (ScopeLineRange{"method", 3, 7}) {
		t.Fatalf("rust impl method: %+v", a)
	}
	goSource := "package p\nfunc run() {\n switch n {\n case 1:\n  work()\n }\n}\n"
	a = CodeLineScopeAnnotations(goSource, "go", map[int]bool{5: true})[5]
	if a.Nearest == nil || *a.Nearest != (ScopeLineRange{"case", 4, 5}) || a.Scope == nil || *a.Scope != (ScopeLineRange{"func", 2, 7}) {
		t.Fatalf("go case: %+v", a)
	}
}

func TestCodeLineScopeAnnotationsErrorsAndBounds(t *testing.T) {
	for _, content := range []string{"func run( {\n", "package p\nfunc broken( {\n// @func:1-9\n"} {
		got := CodeLineScopeAnnotations(content, "go", map[int]bool{1: true, 2: true, 3: true, 100: true})
		for line, a := range got {
			for _, span := range []*ScopeLineRange{a.Nearest, a.Scope} {
				if span != nil && (span.StartLine < 1 || span.StartLine > line || span.EndLine < line || span.EndLine > len(splitLines(content))) {
					t.Fatalf("invalid span %+v on line %d", span, line)
				}
			}
		}
	}
	if got := CodeLineScopeAnnotations(strings.Repeat(" ", 4), "go", nil); len(got) != 0 {
		t.Fatalf("empty hit set: %#v", got)
	}
	// Go accepts a bodyless declaration, so incomplete input can be parsed as
	// a valid one-line declaration plus an adjacent ERROR for the dangling body.
	// That prefix is not a verified function boundary in this source.
	incomplete := "func run() {\n if true {\n"
	if got := CodeLineScopeAnnotations(incomplete, "go", map[int]bool{1: true, 2: true}); len(got) != 0 {
		t.Fatalf("incomplete Go advertised a recovered boundary: %+v", got)
	}
	for _, selector := range []ScopeSelector{{Kind: "func", StartLine: 1, EndLine: 1, Exact: true}, {Kind: "func", StartLine: 1, EndLine: 1}} {
		if span, status := ResolveScopeSelector(incomplete, "go", selector, nil); status != ScopeMissing {
			t.Fatalf("incomplete Go selected %+v with %q", span, status)
		}
	}
}
