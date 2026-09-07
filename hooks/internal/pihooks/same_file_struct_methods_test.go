package pihooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAnalyzeRepositoryAcceptsMethodsInStructFile(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/thing.go": `package a

type thing struct{ x int }

func (t *thing) Get() int       { return t.x }
func (t thing) String() string  { return "" }
func (thing) Bare()             {}
func (*thing) BarePointer()     {}
`,
	})

	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("analyzeRepository() = %#v, want no diagnostics", diagnostics)
	}
}

func TestAnalyzeRepositoryReportsReviveShapedDiagnostic(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/thing.go":   "package a\n\ntype thing struct{ x int }\n",
		"a/methods.go": "package a\n\nfunc (t *thing) Set(x int) { t.x = x }\n",
	})

	diagnostics, err := AnalyzeRepository(root)
	if err != nil {
		t.Fatalf("AnalyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("AnalyzeRepository() returned %d diagnostics, want 1: %#v", len(diagnostics), diagnostics)
	}
	methodFile := filepath.Join(root, "a", "methods.go")
	want := Diagnostic{
		Severity: "error",
		Failure:  "method Set on thing is declared here, but the type lives in thing.go — keep all methods of a struct in the file that declares the type",
		RuleName: "same-file-struct-methods",
		Category: "layout",
		Position: DiagnosticPosition{
			Start: SourcePosition{Filename: methodFile, Offset: 0, Line: 3, Column: 1},
			End:   SourcePosition{Filename: methodFile, Offset: 1, Line: 3, Column: 2},
		},
		Confidence:      1,
		ReplacementLine: "",
	}
	if !reflect.DeepEqual(diagnostics[0], want) {
		t.Errorf("diagnostic mismatch\n got: %#v\nwant: %#v", diagnostics[0], want)
	}

	encoded, err := json.Marshal(diagnostics[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	const wantJSON = `{"Severity":"error","Failure":"method Set on thing is declared here, but the type lives in thing.go — keep all methods of a struct in the file that declares the type","RuleName":"same-file-struct-methods","Category":"layout","Position":{"Start":{"Filename":"` + "PLACEHOLDER" + `","Offset":0,"Line":3,"Column":1},"End":{"Filename":"` + "PLACEHOLDER" + `","Offset":1,"Line":3,"Column":2}},"Confidence":1,"ReplacementLine":""}`
	wantEncoded := []byte(replacePlaceholder(wantJSON, methodFile))
	if !reflect.DeepEqual(encoded, wantEncoded) {
		t.Errorf("JSON shape mismatch\n got: %s\nwant: %s", encoded, wantEncoded)
	}
}

func TestAnalyzeRepositoryReceiverForms(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/types.go": `package a

type list[T any] struct{ values []T }
type pair[A, B any] struct{ left A; right B }
type plain struct{}
`,
		"a/methods.go": `package a

func (l *list[T]) PointerGeneric() {}
func (list[T]) BareGeneric() {}
func (*list[T]) BarePointerGeneric() {}
func (p pair[A, B]) ValueGeneric() {}
func (plain) Bare() {}
func (*plain) BarePointer() {}
`,
	})

	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 6 {
		t.Fatalf("got %d diagnostics, want 6: %#v", len(diagnostics), diagnostics)
	}
	wantLines := []int{3, 4, 5, 6, 7, 8}
	wantFailures := []string{
		"method PointerGeneric on list is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
		"method BareGeneric on list is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
		"method BarePointerGeneric on list is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
		"method ValueGeneric on pair is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
		"method Bare on plain is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
		"method BarePointer on plain is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type",
	}
	for index, diagnostic := range diagnostics {
		if diagnostic.Position.Start.Line != wantLines[index] || diagnostic.Failure != wantFailures[index] {
			t.Errorf("diagnostic[%d] = %#v, want line %d and failure %q", index, diagnostic, wantLines[index], wantFailures[index])
		}
	}
}

func TestAnalyzeRepositoryUsesSyntaxTree(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/types.go": `package a

// type phantom struct{}
var source = "type quoted struct{}"

type (
	grouped struct{}
	number int
)
`,
		"a/methods.go": `package a

// func (p *phantom) Commented() {}
var source = "func (q *quoted) InString() {}"

func (g grouped) Real() {}
func (n number) IgnoredNamedType() {}
`,
	})

	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("got %d diagnostics, want only grouped struct method: %#v", len(diagnostics), diagnostics)
	}
	if diagnostics[0].Failure != "method Real on grouped is declared here, but the type lives in types.go — keep all methods of a struct in the file that declares the type" {
		t.Errorf("unexpected failure: %q", diagnostics[0].Failure)
	}
	if diagnostics[0].Position.Start.Line != 6 {
		t.Errorf("line = %d, want 6", diagnostics[0].Position.Start.Line)
	}
}

func TestAnalyzeRepositoryScopesTypesByDirectory(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/type.go":   "package a\n\ntype thing struct{}\n",
		"b/method.go": "package b\n\nfunc (t thing) OtherDirectory() {}\n",
		"b/type.go":   "package b\n\ntype thing struct{}\n\nfunc (t thing) SameFile() {}\n",
		"c/method.go": "package c\n\nfunc (t thing) NoDeclaration() {}\n",
	})

	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %#v", len(diagnostics), diagnostics)
	}
	if diagnostics[0].Position.Start.Filename != filepath.Join(root, "b", "method.go") {
		t.Errorf("filename = %q, want b/method.go", diagnostics[0].Position.Start.Filename)
	}
}

func TestAnalyzeRepositoryScopesTypesByPackage(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/type.go":          "package a\n\ntype thing struct{}\n",
		"a/external_test.go": "package a_test\n\nfunc (t thing) External() {}\n",
	})
	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("external test package produced diagnostics: %#v", diagnostics)
	}
}

func TestAnalyzeRepositoryIgnoresAmbiguousStructDeclarations(t *testing.T) {
	root := makeRepository(t, map[string]string{
		"a/type_linux.go":   "package a\n\ntype thing struct{}\n",
		"a/type_windows.go": "package a\n\ntype thing struct{}\n",
		"a/method.go":       "package a\n\nfunc (t thing) Method() {}\n",
	})
	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("ambiguous build variants produced diagnostics: %#v", diagnostics)
	}
}

func TestAnalyzeRepositorySkipsNonProjectDirectoriesAndHiddenEntries(t *testing.T) {
	files := map[string]string{
		"kept/type.go":      "package kept\n\ntype kept struct{}\n",
		"kept/method.go":    "package kept\n\nfunc (k kept) Reported() {}\n",
		".hidden/type.go":   "package hidden\n\ntype hidden struct{}\n",
		".hidden/method.go": "package hidden\n\nfunc (h hidden) Ignored() {}\n",
	}
	for _, directory := range []string{"vendor", "examples", "testdata", "node_modules", ".git"} {
		files[directory+"/sample/type.go"] = "package sample\n\ntype sample struct{}\n"
		files[directory+"/sample/method.go"] = "package sample\n\nfunc (s sample) Ignored() {}\n"
	}
	root := makeRepository(t, files)

	diagnostics, err := analyzeRepository(root)
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Failure[:15] != "method Reported" {
		t.Fatalf("got %#v, want only kept.Reported", diagnostics)
	}
}

func TestAnalyzeRepositoryMissingRootIsClean(t *testing.T) {
	diagnostics, err := analyzeRepository(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("analyzeRepository() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("analyzeRepository() = %#v, want no diagnostics", diagnostics)
	}
}

func makeRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}
	return root
}

func replacePlaceholder(value, replacement string) string {
	const placeholder = "PLACEHOLDER"
	for index := 0; index+len(placeholder) <= len(value); index++ {
		if value[index:index+len(placeholder)] == placeholder {
			value = value[:index] + replacement + value[index+len(placeholder):]
			index += len(replacement) - 1
		}
	}
	return value
}
