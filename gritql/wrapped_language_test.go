package gritql

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestWrappedTreeSitterLanguagesEvaluateCalls(t *testing.T) {
	t.Parallel()
	tests := []languageEvaluationCase{
		{name: "c", language: "c", snippet: "target($args)", source: "void run(void) { target(value); }\n", path: "src/app.c", want: "target(value)"},
		{name: "cpp", language: "cpp", snippet: "target($args)", source: "void run() { target(value); }\n", path: "src/app.cpp", want: "target(value)"},
		{name: "csharp", language: "csharp", snippet: "Target($args)", source: "class App { void Run() { Target(value); } }\n", path: "src/App.cs", want: "Target(value)"},
		{name: "java", language: "java", snippet: "target($args)", source: "class App { void run() { target(value); } }\n", path: "src/App.java", want: "target(value)"},
		{name: "kotlin", language: "kotlin", snippet: "target($args)", source: "fun run() { target(value) }\n", path: "src/App.kt", want: "target(value)"},
		{name: "php", language: "php", snippet: "target($args)", source: "<?php function run() { target($value); }\n", path: "src/app.php", want: "target($value)"},
		{name: "rust", language: "rust", snippet: "target($args)", source: "fn run() { target(value); }\n", path: "src/app.rs", want: "target(value)"},
		{name: "shell", language: "shell", snippet: "target $args", source: "run() { target value; }\n", path: "scripts/app.sh", want: "target value"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLanguageEvaluation(t, test)
		})
	}
}

func TestWrappedTreeSitterLanguageConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/tree-sitter-languages/cases.json")
}

func TestWrappedTreeSitterLanguagesUseSharedBindingEquality(t *testing.T) {
	t.Parallel()
	tests := []languageEvaluationCase{
		{name: "c", language: "c", snippet: "pair($value, $value)", source: "void run(void) { pair(foo, foo); pair(foo, bar); }\n", path: "src/app.c", want: "pair(foo, foo)"},
		{name: "cpp", language: "cpp", snippet: "pair($value, $value)", source: "void run() { pair(foo, foo); pair(foo, bar); }\n", path: "src/app.cpp", want: "pair(foo, foo)"},
		{name: "csharp", language: "csharp", snippet: "Pair($value, $value)", source: "class App { void Run() { Pair(foo, foo); Pair(foo, bar); } }\n", path: "src/App.cs", want: "Pair(foo, foo)"},
		{name: "java", language: "java", snippet: "pair($value, $value)", source: "class App { void run() { pair(foo, foo); pair(foo, bar); } }\n", path: "src/App.java", want: "pair(foo, foo)"},
		{name: "kotlin", language: "kotlin", snippet: "pair($value, $value)", source: "fun run() { pair(foo, foo); pair(foo, bar) }\n", path: "src/App.kt", want: "pair(foo, foo)"},
		{name: "php", language: "php", snippet: "pair($value, $value)", source: "<?php pair($foo, $foo); pair($foo, $bar);\n", path: "src/app.php", want: "pair($foo, $foo)"},
		{name: "rust", language: "rust", snippet: "pair($value, $value)", source: "fn run() { pair(foo, foo); pair(foo, bar); }\n", path: "src/app.rs", want: "pair(foo, foo)"},
		{name: "shell", language: "shell", snippet: "pair $value $value", source: "pair foo foo\npair foo bar\n", path: "scripts/app.sh", want: "pair foo foo"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLanguageEvaluation(t, test)
		})
	}
}

func TestWrappedTreeSitterLanguageScannersAndMalformedSources(t *testing.T) {
	t.Parallel()
	tests := []struct {
		language, path, source, malformed string
	}{
		{"c", "src/app.c", "void run(void) { target(value); }\n", "void broken(\n"},
		{"cpp", "src/app.cpp", "void run() { target(value); }\n", "void broken(\n"},
		{"csharp", "src/App.cs", "class App { void Run() { Target(value); } }\n", "class Broken { void Run(\n"},
		{"java", "src/App.java", "class App { void run() { target(value); } }\n", "class Broken { void run(\n"},
		{"kotlin", "src/App.kt", "fun run() { target(value) }\n", "fun broken(\n"},
		{"php", "src/app.php", "<?php target($value);\n", "<?php function broken(\n"},
		{"rust", "src/app.rs", "fn run() { target(value); }\n", "fn broken(\n"},
		{"shell", "scripts/app.sh", "target value\n", "if then\n"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.language, func(t *testing.T) {
			t.Parallel()
			program := compileWrappedLanguageScannerPattern(t, test.language)
			filesystem := fstest.MapFS{test.path: {Data: []byte(test.source)}}
			result := ScanFiles(context.Background(), filesystem, program, []ScanCandidate{{ReadPath: test.path, Path: test.path}}, ScanOptions{Workers: 1})
			if len(result.Findings()) != 1 || len(result.Diagnostics()) != 0 {
				t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
			}
			malformed := EvaluateFile(context.Background(), program, FileInput{Path: test.path, Language: test.language, Content: []byte(test.malformed)}, EvaluateOptions{})
			if len(malformed.Findings()) != 0 || len(malformed.Diagnostics()) != 1 || malformed.Diagnostics()[0].Code() != "SOURCE_PARSE" {
				t.Fatalf("malformed findings=%v diagnostics=%v", malformed.Findings(), malformed.Diagnostics())
			}
		})
	}
}

func compileWrappedLanguageScannerPattern(t *testing.T, language string) *Program {
	t.Helper()
	snippet := "target($value)"
	if language == "csharp" {
		snippet = "Target($value)"
	} else if language == "shell" {
		snippet = "target $value"
	}
	program, err := Compile([]byte("language "+language+"\n`"+snippet+"`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return program
}
