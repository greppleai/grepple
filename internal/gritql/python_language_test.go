package gritql

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestPythonStructuralEvaluation(t *testing.T) {
	t.Parallel()
	tests := []languageEvaluationCase{
		{name: "call", language: "python", snippet: "target($args)", source: "value = target(one, two)\n", path: "src/app.py", want: "target(one, two)"},
		{name: "keyword argument", language: "python", snippet: "target(name=$value)", source: "result = target(name=item)\n", path: "src/app.py", want: "target(name=item)"},
		{name: "assignment", language: "python", snippet: "$name = $value", source: "answer = 42\n", path: "src/app.py", want: "answer = 42"},
		{name: "decorated declaration", language: "python", snippet: "@route($path)\ndef handler($args):\n    $body", source: "@route('/items')\ndef handler(request):\n    return load(request)\n", path: "src/app.py", want: "@route('/items')\ndef handler(request):\n    return load(request)"},
		{name: "statement sequence", language: "python", snippet: "first()\nsecond()", source: "def run():\n    first()\n    second()\n    third()\n", path: "src/app.py", want: "first()\n    second()"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLanguageEvaluation(t, test)
		})
	}
}

func TestPythonWholePlaceholderRetainsExpressionStatementAndDeclarationContexts(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language python\n`$node`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[SnippetContext]bool{}
	for _, template := range program.Root().Templates() {
		if template.RootSlot().Valid() {
			contexts[template.Context()] = true
		}
	}
	for _, context := range []SnippetContext{SnippetContextExpression, SnippetContextStatement, SnippetContextDeclaration} {
		if !contexts[context] {
			t.Fatalf("root-slot contexts=%v, missing %s", contexts, context)
		}
	}
}

func TestPythonScannerAndMetadata(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language python\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filesystem := fstest.MapFS{
		"src/app.py":  {Data: []byte("target(one)\n")},
		"src/app.pyi": {Data: []byte("target(two)\n")},
		"src/app.pyw": {Data: []byte("target(three)\n")},
		"src/app.go":  {Data: []byte("package p\nfunc f(){ target(four) }\n")},
	}
	candidates := []ScanCandidate{
		{ReadPath: "src/app.py", Path: "src/app.py"},
		{ReadPath: "src/app.pyi", Path: "src/app.pyi"},
		{ReadPath: "src/app.pyw", Path: "src/app.pyw"},
		{ReadPath: "src/app.go", Path: "src/app.go"},
	}
	result := ScanFiles(context.Background(), filesystem, program, candidates, ScanOptions{Workers: 1})
	if len(result.Findings()) != 3 || result.Stats().SkippedLanguage != 1 {
		t.Fatalf("findings=%d stats=%#v diagnostics=%v", len(result.Findings()), result.Stats(), result.Diagnostics())
	}
	metadata := result.Metadata()
	if metadata.Contract != Compatibility || metadata.Language != "python" || metadata.Grammar != PythonGrammar || metadata.TreeSitterGrammar != TreeSitterPythonGrammar {
		t.Fatalf("metadata=%#v", metadata)
	}
}

func TestPythonMalformedSourceIsTransactional(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language python\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "src/app.py", Language: "python", Content: []byte("def broken(\n")}, EvaluateOptions{})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
}
