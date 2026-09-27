package gritql

import (
	"context"
	"fmt"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestNodeLikeNamedFieldAcrossSupportedLanguages(t *testing.T) {
	cases := []struct{ language, source string }{
		{"c", "int value = 1;\n"}, {"cpp", "int value = 1;\n"},
		{"csharp", "class App { int value = 1; }\n"},
		{"dart", "void run(int value) {}\n"},
		{"go", "package demo\nvar value = 1\n"},
		{"java", "class App { int value = 1; }\n"},
		{"javascript", "const value = 1;\n"},
		{"kotlin", "fun run(value: Int): Int = value\n"},
		{"php", "<?php function run($value) { return $value; }\n"},
		{"python", "value = 1\n"},
		{"rust", "fn run() { let value = 1; }\n"},
		{"shell", "value=1\n"},
		{"tsx", "const value = 1;\n"},
		{"typescript", "const value = 1;\n"},
	}
	if len(cases) != len(SupportedLanguages()) {
		t.Fatalf("sources=%d languages=%d", len(cases), len(SupportedLanguages()))
	}
	seen := make(map[string]bool, len(cases))
	for _, test := range cases {
		if seen[test.language] {
			t.Fatalf("duplicate language %q", test.language)
		}
		seen[test.language] = true
	}
	for _, supported := range SupportedLanguages() {
		if !seen[supported.ID] {
			t.Fatalf("missing named-field test for %q", supported.ID)
		}
	}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			document, err := parser.ParseDocument(test.language, test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer document.Close()
			var parent parser.Node
			field := ""
			stack := []parser.Node{document.Root()}
			for len(stack) > 0 && field == "" {
				item := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for _, child := range item.NamedChildren() {
					candidate := child.FieldName()
					if !child.IsExtra() && validNodeSelectorName(candidate) && parser.GrammarFieldCardinality(test.language, item.Kind(), candidate) != parser.GrammarCardinalityUnknown {
						parent, field = item, candidate
						break
					}
					stack = append(stack, child)
				}
			}
			if field == "" {
				t.Fatalf("%s: no declared named grammar field", test.language)
			}
			query := fmt.Sprintf("language %s\n%s(%s=$captured)", test.language, parent.Kind(), field)
			program, err := Compile([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			result := EvaluateFile(context.Background(), program, FileInput{Path: "source", Language: test.language, Content: []byte(test.source)}, EvaluateOptions{})
			if len(result.Diagnostics()) != 0 || len(result.Findings()) == 0 {
				t.Fatalf("query=%s findings=%v diagnostics=%v", query, result.Findings(), result.Diagnostics())
			}
		})
	}
}
