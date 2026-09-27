package gritql

import (
	"context"
	"strings"
	"testing"
)

func TestEmptyConstraintEverySupportedLanguage(t *testing.T) {
	cases := []struct {
		language, snippet, source string
	}{
		{"c", "target($args)", "void run(void) { target(); target(value); }\n"},
		{"cpp", "target($args)", "void run() { target(); target(value); }\n"},
		{"csharp", "Target($args)", "class App { void Run() { Target(); Target(value); } }\n"},
		{"go", "target($args)", "package demo\nfunc run() { target(); target(value) }\n"},
		{"java", "target($args)", "class App { void run() { target(); target(value); } }\n"},
		{"javascript", "target($args)", "function run() { target(); target(value); }\n"},
		{"kotlin", "target($args)", "fun run() { target(); target(value) }\n"},
		{"python", "target($args)", "target()\ntarget(value)\n"},
		{"rust", "target($args)", "fn run() { target(); target(value); }\n"},
		{"shell", "target $args", "target\ntarget value\n"},
		{"tsx", "target($args)", "function run() { target(); target(value); }\n"},
		{"typescript", "target($args)", "function run() { target(); target(value); }\n"},
	}
	seen := make(map[string]bool, len(cases))
	for _, test := range cases {
		if seen[test.language] {
			t.Fatalf("duplicate test language %q", test.language)
		}
		seen[test.language] = true
	}
	for _, supported := range SupportedLanguages() {
		if !seen[supported.ID] {
			t.Fatalf("missing language %q", supported.ID)
		}
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			query := "language " + test.language + "\n`" + test.snippet + "` where { $args <: empty }"
			program, err := Compile([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result := EvaluateFile(context.Background(), program, FileInput{Path: "src/sample", Language: test.language, Content: []byte(test.source)}, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 {
				t.Fatalf("diagnostics=%v", result.Diagnostics())
			}
			findings := result.Findings()
			if len(findings) != 1 || !strings.Contains(findings[0].Text(), "target") && !strings.Contains(findings[0].Text(), "Target") || strings.Contains(findings[0].Text(), "value") {
				t.Fatalf("findings=%v want one zero-argument call", findings)
			}
		})
	}
}

// In languages whose adapters expose a zero-element statement-list slot,
// also verify empty and non-empty brace bodies. Other languages are covered
// by the argument-list test above; empty syntax is language-specific.
func TestEmptyConstraintBraceBodies(t *testing.T) {
	cases := []struct{ language, snippet, source string }{
		{"go", "for { $body }", "package demo\nfunc run() { for {}\nfor { target() } }"},
		{"javascript", "for (;;) { $body }", "function run() { for (;;) {} for (;;) { target(); } }"},
		{"kotlin", "while (true) { $body }", "fun run() { while (true) {}\nwhile (true) { target() } }"},
		{"rust", "loop { $body }", "fn run() { loop {} loop { target(); } }"},
		{"tsx", "for (;;) { $body }", "function run() { for (;;) {} for (;;) { target(); } }"},
		{"typescript", "for (;;) { $body }", "function run() { for (;;) {} for (;;) { target(); } }"},
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			query := "language " + test.language + "\n`" + test.snippet + "` where { $body <: empty }"
			program, err := Compile([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result := EvaluateFile(context.Background(), program, FileInput{Path: "src/sample", Language: test.language, Content: []byte(test.source)}, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 || len(result.Findings()) != 1 || strings.Contains(result.Findings()[0].Text(), "target") || strings.Contains(result.Findings()[0].Text(), "Target") {
				t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
			}
		})
	}
}
func TestEmptyConstraintCompilerContext(t *testing.T) {
	for _, source := range []string{
		"language go\nempty",
		"language go\nnot empty",
		"language go\n`f($x)` where { $x <: not empty }",
	} {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_PARSE")
	}
	program, err := Compile([]byte("language go\n`f($x)` where { $x <: empty }"), CompileOptions{})
	if err != nil || program.Root().Constraints()[0].RHS().Kind() != KindEmpty || !program.Features().Has(FeatureEmpty) {
		t.Fatalf("program=%v error=%v", program, err)
	}
}

func TestEmptyConstraintRejectsScalarNode(t *testing.T) {
	program, err := Compile([]byte("language go\n`$value + 1` where { $value <: empty }"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "sample.go", Content: []byte("package demo\nvar _ = value + 1\n")}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 0 {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
}
