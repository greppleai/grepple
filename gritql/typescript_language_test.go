package gritql

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

type languageEvaluationCase struct {
	name, language, snippet, source, path, want string
}
type typeScriptConformanceCase struct {
	Name, Language, Query, Path, Source string
	Findings                            []string
}

func TestTypeScriptAndTSXStructuralEvaluation(t *testing.T) {
	t.Parallel()
	tests := []languageEvaluationCase{
		{name: "call", language: "typescript", snippet: "foo($args)", source: "const value = foo(1, item);\n", path: "src/app.ts", want: "foo(1, item)"},
		{name: "type interpretation", language: "typescript", snippet: "Promise<$type>", source: "type Result = Promise<string>;\n", path: "src/types.mts", want: "Promise<string>"},
		{name: "object property", language: "typescript", snippet: "{$key: $value}", source: "const value = {answer: 42};\n", path: "src/app.cts", want: "{answer: 42}"},
		{name: "import source", language: "typescript", snippet: "import {value} from $source;", source: "import {value} from \"pkg\";\n", path: "src/app.ts", want: "import {value} from \"pkg\";"},
		{name: "typed declaration", language: "typescript", snippet: "function f($name: $type) {}", source: "function f(name: string) {}\n", path: "src/app.ts", want: "function f(name: string) {}"},
		{name: "statement sequence", language: "typescript", snippet: "first(); second();", source: "function f(){ first(); second(); third(); }\n", path: "src/app.ts", want: "first(); second();"},
		{name: "declaration sequence", language: "typescript", snippet: "interface A {}\ntype B = string;", source: "interface A {}\ntype B = string;\nconst c = 1;\n", path: "src/app.ts", want: "interface A {}\ntype B = string;"},
		{name: "tsx", language: "tsx", snippet: "<Button value={$value} />", source: "const view = <Button value={item} />;\n", path: "src/view.tsx", want: "<Button value={item} />"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLanguageEvaluation(t, test)
		})
	}
}

func assertLanguageEvaluation(t *testing.T, test languageEvaluationCase) {
	t.Helper()
	program, err := Compile([]byte("language "+test.language+"\n`"+test.snippet+"`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if program.Language() != test.language || program.Compatibility() != Compatibility {
		t.Fatalf("program language=%q compatibility=%q", program.Language(), program.Compatibility())
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.path, Content: []byte(test.source)}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	findings := result.Findings()
	if len(findings) != 1 || findings[0].Text() != test.want {
		t.Fatalf("findings=%v, want %q", findings, test.want)
	}
}
func TestTypeScriptConformanceFixture(t *testing.T) {
	content, err := os.ReadFile("testdata/conformance/typescript/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []typeScriptConformanceCase
	if err := json.Unmarshal(content, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("TypeScript conformance fixture is empty")
	}
	for _, test := range cases {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			assertTypeScriptConformanceCase(t, test)
		})
	}
}
func assertTypeScriptConformanceCase(t *testing.T, test typeScriptConformanceCase) {
	t.Helper()
	program, err := Compile([]byte(test.Query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.Path, Language: test.Language, Content: []byte(test.Source)}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	findings := result.Findings()
	texts := make([]string, len(findings))
	for index := range findings {
		texts[index] = findings[index].Text()
	}
	if !slices.Equal(texts, test.Findings) {
		t.Fatalf("finding texts=%q want %q", texts, test.Findings)
	}
}

func TestTypeScriptUsesSharedQueryAlgebraAndBindingEquality(t *testing.T) {
	t.Parallel()
	query := "language typescript\nand { `pair($value, $value)`, contains `foo` }"
	program, err := Compile([]byte(query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "const yes = pair(foo, foo);\nconst no = pair(foo, bar);\n"
	result := EvaluateFile(context.Background(), program, FileInput{Path: "src/app.ts", Language: "typescript", Content: []byte(source)}, EvaluateOptions{})
	findings := result.Findings()
	if len(result.Diagnostics()) != 0 || len(findings) != 1 || findings[0].Text() != "pair(foo, foo)" {
		t.Fatalf("findings=%v diagnostics=%v", findings, result.Diagnostics())
	}
}

func TestTypeScriptMalformedQueryNamesUnifiedContract(t *testing.T) {
	t.Parallel()
	_, err := Compile([]byte("language typescript\n`unterminated"), CompileOptions{})
	if err == nil || !strings.Contains(err.Error(), Compatibility) {
		t.Fatalf("compile error=%v", err)
	}
}

func TestTypeScriptAmbiguousSnippetRetainsExpressionAndTypeTemplates(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`Promise<$type>`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	templates := program.Root().Templates()
	if len(templates) < 2 {
		t.Fatalf("templates=%d, want expression and type interpretations", len(templates))
	}
	contexts := make(map[SnippetContext]bool)
	for _, template := range templates {
		contexts[template.Context()] = true
	}
	if !contexts[SnippetContextExpression] || !contexts[SnippetContextType] {
		t.Fatalf("template contexts=%v", contexts)
	}
}

func TestTypeScriptScannerDetectsCanonicalExtensions(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filesystem := fstest.MapFS{
		"src/app.ts":  &fstest.MapFile{Data: []byte("target(one);\n")},
		"src/app.mts": &fstest.MapFile{Data: []byte("target(two);\n")},
		"src/app.go":  &fstest.MapFile{Data: []byte("package p\nfunc f(){ target(three) }\n")},
	}
	candidates := []ScanCandidate{{ReadPath: "src/app.ts", Path: "src/app.ts"}, {ReadPath: "src/app.mts", Path: "src/app.mts"}, {ReadPath: "src/app.go", Path: "src/app.go"}}
	result := ScanFiles(context.Background(), filesystem, program, candidates, ScanOptions{Workers: 1})
	if len(result.Findings()) != 2 || result.Stats().SkippedLanguage != 1 {
		t.Fatalf("findings=%d stats=%#v diagnostics=%v", len(result.Findings()), result.Stats(), result.Diagnostics())
	}
	metadata := result.Metadata()
	if metadata.Contract != Compatibility || metadata.Language != "typescript" || metadata.Grammar != TypeScriptGrammar || metadata.GoGrammar != "" || metadata.TreeSitterGrammar != TreeSitterTypeScriptGrammar {
		t.Fatalf("metadata=%#v", metadata)
	}
}
func TestTypeScriptMalformedSourceIsTransactional(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "src/app.ts", Language: "typescript", Content: []byte("function broken(\n")}, EvaluateOptions{})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
}

func TestTypeScriptProgramRejectsMismatchedDocumentLanguage(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`value`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "main.go", Language: "go", Content: []byte("package p\nvar value = 1\n")}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 1 || diagnostics[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}
