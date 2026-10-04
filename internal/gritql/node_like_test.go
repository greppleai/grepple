package gritql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestNodeLikeCapturesGroupedGoTypeSpecs(t *testing.T) {
	program, err := Compile([]byte("language go\ntype_spec(name=$name, type=struct_type())"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("package demo\ntype (\n  first struct{}\n  notStruct int\n  second struct{ n int }\n)\n")
	result := EvaluateFile(context.Background(), program, FileInput{Path: "sample.go", Language: "go", Content: content}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 {
		t.Fatalf("diagnostics: %v", result.Diagnostics())
	}
	findings := result.Findings()
	if len(findings) != 2 {
		t.Fatalf("findings=%v want two struct type specs", findings)
	}
	for index, want := range []string{"first", "second"} {
		bindings := findings[index].Bindings()
		if len(bindings) != 1 {
			t.Fatalf("finding[%d] bindings=%v", index, bindings)
		}
		node, ok := bindings[0].Node()
		if bindings[0].Name() != "name" || !ok || node.Lexeme() != want {
			t.Fatalf("finding[%d] bindings=%v want name=%q", index, bindings, want)
		}
	}
}

func TestNodeLikePositionalCaptureAcrossSupportedLanguages(t *testing.T) {
	cases := []struct{ language, source string }{
		{"c", "int value = 1;\n"},
		{"cpp", "int value = 1;\n"},
		{"csharp", "class App { int value = 1; }\n"},
		{"dart", "void run(int value) {}\n"},
		{"hcl", "locals { value = 1 }\n"},
		{"svelte", "<button>value</button>\n"},
		{"go", "package demo\nvar value = 1\n"},
		{"java", "class App { int value = 1; }\n"},
		{"javascript", "const value = 1;\n"},
		{"kotlin", "val value = 1\n"},
		{"php", "<?php function run($value) { return $value; }\n"},
		{"python", "value = 1\n"},
		{"rust", "fn run() { let value = 1; }\n"},
		{"shell", "value=1\n"},
		{"swift", "func run(value: Int) {}\n"},
		{"tsx", "const value = 1;\n"},
		{"typescript", "const value = 1;\n"},
	}
	if len(cases) != len(SupportedLanguages()) {
		t.Fatalf("test cases=%d supported=%d", len(cases), len(SupportedLanguages()))
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			document, err := parser.NewParser().Parse(test.language, test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer document.Close()
			root := document.Root()
			children := root.NamedChildren()
			if len(children) == 0 {
				t.Fatalf("%s has no named children", root.Kind())
			}
			query := fmt.Sprintf("language %s\n%s($first)", test.language, root.Kind())
			program, err := Compile([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatalf("compile %q: %v", query, err)
			}
			result := EvaluateFile(context.Background(), program, FileInput{Path: "source", Language: test.language, Content: []byte(test.source)}, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 || len(result.Findings()) == 0 {
				t.Fatalf("diagnostics=%v findings=%v", result.Diagnostics(), result.Findings())
			}
			first := result.Findings()[0].Bindings()
			if len(first) != 1 || first[0].Name() != "first" {
				t.Fatalf("bindings=%v", first)
			}
			node, ok := first[0].Node()
			if !ok || node.Kind() != children[0].Kind() {
				t.Fatalf("node=%s want=%s", node.Kind(), children[0].Kind())
			}
		})
	}
}

func TestNodeLikeGoMethodsAndPackage(t *testing.T) {
	content := []byte("package demo\ntype list[T any] struct{}\nfunc (l *list[T]) Generic() {}\nfunc (*list[T]) Pointer() {}\nfunc (list[T]) Value() {}\nfunc (l list[T]) Named() {}\n")
	for _, test := range []struct {
		query string
		count int
	}{
		{"package_clause($package)", 1},
		{"method_declaration(name=$method, receiver=parameter_list(parameter_declaration(type=$receiver)))", 4},
	} {
		program, err := Compile([]byte("language go\n"+test.query), CompileOptions{})
		if err != nil {
			t.Fatalf("%s: %v", test.query, err)
		}
		result := EvaluateFile(context.Background(), program, FileInput{Path: "sample.go", Language: "go", Content: content}, EvaluateOptions{})
		if len(result.Diagnostics()) != 0 || len(result.Findings()) != test.count {
			t.Fatalf("%s: diagnostics=%v findings=%v", test.query, result.Diagnostics(), result.Findings())
		}
	}
}

func TestNodeLikeSkipsCommentAndOrdersPositionalChildren(t *testing.T) {
	program, err := Compile([]byte("language javascript\nprogram($first, $second)"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("// trivia\nconst first=1; const second=2;\n")
	result := EvaluateFile(context.Background(), program, FileInput{Path: "sample.js", Language: "javascript", Content: source}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 1 {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
	found := map[string]string{}
	for _, binding := range result.Findings()[0].Bindings() {
		node, ok := binding.Node()
		if !ok || node.Kind() != "lexical_declaration" {
			t.Fatalf("binding=%+v node=%s", binding, node.Kind())
		}
		found[binding.Name()] = string(source[binding.Range().StartByte:binding.Range().EndByte])
	}
	if !strings.Contains(found["first"], "first") || !strings.Contains(found["second"], "second") {
		t.Fatalf("bindings=%v", found)
	}
}

func TestNodeLikeRejectsUnknownKindsAndFields(t *testing.T) {
	for _, query := range []string{
		"language go\nnot_a_go_node()",
		"language go\ntype_spec(unknown=$name)",
		"language typescript\nvariable_declarator(unknown=$name)",
	} {
		_, err := Compile([]byte(query), CompileOptions{})
		var compileErr *CompileError
		if !errors.As(err, &compileErr) || compileErr.Code != "PATTERN_INVALID_SNIPPET" {
			t.Fatalf("query=%q error=%v", query, err)
		}
	}
}
