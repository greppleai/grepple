package gritql

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

const authoredGoMcCabe = `language go
{ metric: {
  scope: or { function_declaration(), method_declaration() },
  name: "name",
  base: 1,
  above: 3,
  boundary: func_literal(),
  rules: [
    {id: "if", query: if_statement(), points: 1},
    {id: "loop", query: for_statement(), points: 1},
    {id: "arm", query: or { expression_case(), type_case(), communication_case() }, points: 1},
    {id: "short-circuit", query: binary_expression(), points: 1, logical: "each", operators: ["&&", "||"]}
  ]
} }`

const authoredGoCognitive = `language go
{ metric: {
  scope: or { function_declaration(), method_declaration() },
  name: "name", base: 0, above: 15,
  rules: [
    {id: "if", query: if_statement(), points: 1, depth: 1, opens: true, flat: "alternative"},
    {id: "loop", query: for_statement(), points: 1, depth: 1, opens: true},
    {id: "switch", query: or { expression_switch_statement(), type_switch_statement(), select_statement() }, points: 1, depth: 1, opens: true},
    {id: "closure", query: func_literal(), points: 0, depth: 1, opens: true},
    {id: "logical", query: binary_expression(), points: 1, logical: "runs", operators: ["&&", "||"]},
    {id: "label", query: or { break_statement(), continue_statement(), goto_statement() }, points: 1, child: "label_name"},
    {id: "recursion", query: call_expression(), points: 1, self: "function"}
  ]
} }`

func TestAuthoredGritQLCalculatesMcCabeAndCognitive(t *testing.T) {
	for _, test := range []struct {
		query        string
		score, above int
	}{{authoredGoMcCabe, 5, 3}, {authoredGoCognitive, 3, 15}} {
		compiled, err := CompileMetric([]byte(test.query), CompileOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if compiled.Above != test.above {
			t.Fatalf("threshold=%d", compiled.Above)
		}
		doc, err := parser.ParseDocument("go", "package demo\nfunc f(a,b,c,d bool){if a && b && c || d {}}\n")
		if err != nil {
			t.Fatal(err)
		}
		results, err := AnalyzeMetrics(context.Background(), compiled.Spec, doc, EvaluateOptions{})
		doc.Close()
		if err != nil || len(results) != 1 || results[0].Name != "f" || results[0].Score != test.score {
			t.Fatalf("scores=%v error=%v want %d", results, err, test.score)
		}
	}
}

func TestAuthoredMetricRejectsInvalidConfigBeforeNoFinding(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"unknown field", strings.Replace(authoredGoMcCabe, `name: "name"`, `name: "not_a_go_field"`, 1)},
		{"unknown selector", strings.Replace(authoredGoMcCabe, "if_statement()", "made_up_ast_node()", 1)},
		{"unknown key", strings.Replace(authoredGoMcCabe, "above: 3", "abv: 3", 1)},
		{"duplicate key", strings.Replace(authoredGoMcCabe, "above: 3", "above: 3, above: 3", 1)},
		{"invalid mode", strings.Replace(authoredGoMcCabe, `logical: "each"`, `logical: "jump"`, 1)},
		{"unknown operator", strings.Replace(authoredGoMcCabe, `"&&", "||"`, `"&&&", "||"`, 1)},
		{"malformed list", strings.Replace(authoredGoMcCabe, "points: 1, logical:", "points: false, logical:", 1)},
		{"empty rules", strings.Replace(authoredGoMcCabe, `rules: [`, `rules: [`, 1) + ` `},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "empty rules" {
				test.query = strings.Replace(authoredGoMcCabe, `rules: [
    {id: "if", query: if_statement(), points: 1},
    {id: "loop", query: for_statement(), points: 1},
    {id: "arm", query: or { expression_case(), type_case(), communication_case() }, points: 1},
    {id: "short-circuit", query: binary_expression(), points: 1, logical: "each", operators: ["&&", "||"]}
  ]`, `rules: []`, 1)
			}
			_, err := CompileMetric([]byte(test.query), CompileOptions{})
			var compileErr *CompileError
			if err == nil || !errors.As(err, &compileErr) {
				t.Fatalf("invalid metric accepted: %v", err)
			}
		})
	}
}

const authoredNestedLoops = `language go
{ metric: {
  scope: or { function_declaration(), method_declaration() },
  name: "name", base: 0, above: 1,
  boundary: func_literal(),
  rules: [{id: "nested-loop", query: for_statement(), points: 0, depth: 1, opens: true}]
} }`

func TestAuthoredNestedLoopMetricScoresOnlyNestedLoops(t *testing.T) {
	compiled, err := CompileMetric([]byte(authoredNestedLoops), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Above != 1 {
		t.Fatalf("nested-loop threshold=%d, want 1", compiled.Above)
	}
	const source = `package demo
func nested() { for range a { for range b {} } }
func siblings() { for range a {}; for range b {} }
func twoPairs() { for range a { for range b {} }; for range a { for range b {} } }
func twoInOne() { for range a { for range b {}; for range c {} } }
func triple() { for range a { for range b { for range c {} } } }
func throughIf() { for range a { if true { for range b {} } } }
func closure() { for range a { _ = func() { for range b {} } } }
func separate() { for range a {}; _ = func() { for range b {} } }
`
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	results, err := AnalyzeMetrics(context.Background(), compiled.Spec, doc, EvaluateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"nested": 1, "siblings": 0, "twoPairs": 2, "twoInOne": 2, "triple": 3, "throughIf": 1, "closure": 0, "separate": 0}
	if len(results) != len(want) {
		t.Fatalf("got %d scores, want %d: %+v", len(results), len(want), results)
	}
	for _, result := range results {
		score, exists := want[result.Name]
		if !exists || result.Score != score {
			t.Fatalf("result %+v want %d exists=%v", result, score, exists)
		}
		delete(want, result.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing function scores: %v", want)
	}
}
