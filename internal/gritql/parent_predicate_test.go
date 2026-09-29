package gritql

import (
	"context"
	"strings"
	"testing"
)

func TestDirectFunctionParentAcrossEverySupportedLanguage(t *testing.T) {
	cases := []struct{ language, snippet, source string }{
		{"c", "target($args)", "void run(void) { target(value); }\n"},
		{"cpp", "target($args)", "void run() { target(value); }\n"},
		{"csharp", "Target($args)", "class App { void Run() { Target(value); } }\n"},
		{"dart", "target($args)", "void run() { target(value); }\n"},
		{"go", "target($args)", "package demo\nfunc run() { target(value) }\n"},
		{"java", "target($args)", "class App { void run() { target(value); } }\n"},
		{"javascript", "target($args)", "function run() { target(value); }\n"},
		{"kotlin", "target($args)", "fun run() { target(value) }\n"},
		{"php", "target($args)", "<?php function run() { target(value); }\n"},
		{"python", "target($args)", "def run():\n    target(value)\n"},
		{"rust", "target($args)", "fn run() { target(value); }\n"},
		{"shell", "target $args", "run() { target value; }\n"},
		{"swift", "target($args)", "func run() { target(value) }\n"},
		{"tsx", "target($args)", "function run() { target(value); }\n"},
		{"typescript", "target($args)", "function run() { target(value); }\n"},
	}
	seen := make(map[string]bool, len(cases))
	for _, test := range cases {
		if seen[test.language] {
			t.Fatalf("duplicate test language %q", test.language)
		}
		seen[test.language] = true
	}
	for _, supported := range SupportedLanguages() {
		if supported.ID == "hcl" { // HCL has function calls, but no callable declaration bodies.
			continue
		}
		if !seen[supported.ID] {
			t.Fatalf("missing language %q", supported.ID)
		}
	}
	if len(seen) != len(SupportedLanguages())-1 {
		t.Fatalf("function-parent cases=%d, want %d languages with function bodies", len(seen), len(SupportedLanguages())-1)
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			program, err := Compile([]byte("language "+test.language+"\nparent kind(\"function\")"), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if program.Root().Kind() != KindParent || !program.Features().Has(FeatureParent) {
				t.Fatalf("parent kind=%s features=%b", program.Root().Kind(), program.Features())
			}
			input := FileInput{Path: "src/example", Language: test.language, Content: []byte(test.source)}
			result := EvaluateFile(context.Background(), program, input, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 {
				t.Fatalf("diagnostics=%v", result.Diagnostics())
			}
			foundBody := false
			for _, finding := range result.Findings() {
				if strings.Contains(finding.Text(), "target") || strings.Contains(finding.Text(), "Target") {
					foundBody = true
				}
			}
			if !foundBody {
				t.Fatalf("no direct function-body candidate: findings=%v", result.Findings())
			}
			query := "language " + test.language + "\nand { `" + test.snippet + "`, not parent kind(\"function\") }"
			negative, err := Compile([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result = EvaluateFile(context.Background(), negative, input, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 || len(result.Findings()) != 1 {
				t.Fatalf("call has a non-function direct parent: findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
			}
		})
	}
}

func TestDirectFunctionParentAnonymousForms(t *testing.T) {
	cases := []struct{ language, source string }{
		{"cpp", "void run() { auto callback = []() { target(); }; }"},
		{"csharp", "class App { void Run() { System.Action callback = () => { Target(); }; } }"},
		{"go", "package demo\nfunc run() { _ = func() { target() } }"},
		{"java", "class App { void run() { Runnable callback = () -> { target(); }; } }"},
		{"javascript", "const callback = () => { target(); };"},
		{"kotlin", "val callback = { target() }"},
		{"python", "callback = lambda: target()"},
		{"rust", "fn run() { let callback = || { target(); }; }"},
		{"tsx", "const callback = () => { target(); };"},
		{"typescript", "const callback = () => { target(); };"},
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			program, err := Compile([]byte("language "+test.language+"\nparent kind(\"function\")"), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result := EvaluateFile(context.Background(), program, FileInput{Path: "src/example", Language: test.language, Content: []byte(test.source)}, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 {
				t.Fatalf("diagnostics=%v", result.Diagnostics())
			}
			foundBody := false
			for _, finding := range result.Findings() {
				if strings.Contains(finding.Text(), "target()") || strings.Contains(finding.Text(), "Target()") {
					foundBody = true
				}
			}
			if !foundBody {
				t.Fatalf("no anonymous function body candidate: findings=%v", result.Findings())
			}
		})
	}
}

func TestDirectFunctionParentFiltersGoEmptyBlocks(t *testing.T) {
	const source = `package sample
func empty() {}
func run() {
    for {}
    if true {}
    _ = func() {}
}
`
	program, err := Compile([]byte("language go\nand { `{}`, not parent kind(\"function\") }"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "sample.go", Content: []byte(source)}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 2 {
		t.Fatalf("control-flow blocks=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
	for _, finding := range result.Findings() {
		if finding.Range().Start.Line != 4 && finding.Range().Start.Line != 5 {
			t.Fatalf("function body incorrectly reported: %v", result.Findings())
		}
	}
	functionBodies, err := Compile([]byte("language go\nand { `{}`, parent kind(\"function\") }"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result = EvaluateFile(context.Background(), functionBodies, FileInput{Path: "sample.go", Content: []byte(source)}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 2 || result.Findings()[0].Range().Start.Line != 2 || result.Findings()[1].Range().Start.Line != 6 {
		t.Fatalf("named and anonymous function bodies=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
}

func TestFunctionParentRejectsUnknownCategory(t *testing.T) {
	for _, test := range []struct{ query, code string }{
		{"language go\nparent kind(\"loop\")", "PATTERN_UNSUPPORTED"},
		{"language go\nparent kind(\"function\" ) garbage", "PATTERN_PARSE"},
	} {
		_, err := Compile([]byte(test.query), CompileOptions{})
		assertCompileCode(t, err, test.code)
	}
}
