package gritql

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestGoComplexityScores(t *testing.T) {
	cases := []struct {
		name, source          string
		cyclomatic, cognitive int
	}{
		{"straight-line", "func f() { _ = 1 }", 1, 0},
		{"nested", "func f(x int) { if x>0 { for x>0 {x--} } }", 3, 3},
		{"else-if", "func f(x int) { if x>0 {} else if x<0 {} }", 3, 2},
		{"logical-runs", "func f(a,b,c,d bool) { if a && b && c || d {} }", 5, 3},
		{"switch-arms", "func f(x int) { switch x {case 1,2:; case 3:;default:} }", 3, 1},
		{"labeled", "func f() { Outer: for {break Outer} }", 2, 2},
		{"nested-closure", "func f() { _ = func() { if true {} } }", 1, 2},
		{"recursion", "func f() { f() }", 1, 1},
		{"unparenthesized-boolean", "func f(a,b,c bool) { if a && b && c {} }", 4, 2},
		{"parenthesized-boolean", "func f(a,b,c bool) { if a && (b && c) {} }", 4, 3},
		{"type-switch-select", "func f(x any, ch chan int) { switch x.(type) {case int:; default:}; select {case <-ch:; default:} }", 3, 2},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, err := parser.NewParser().Parse("go", "package demo\n"+test.source+"\n")
			if err != nil {
				t.Fatal(err)
			}
			defer document.Close()
			cyclomatic, err := GoCyclomaticMetricSpec()
			if err != nil {
				t.Fatal(err)
			}
			cognitive, err := GoCognitiveMetricSpec()
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range []struct {
				spec MetricSpec
				want int
			}{{cyclomatic, test.cyclomatic}, {cognitive, test.cognitive}} {
				results, err := AnalyzeMetrics(context.Background(), check.spec, document, EvaluateOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if len(results) != 1 || results[0].Score != check.want || results[0].Name != "f" {
					t.Fatalf("score=%v want %d for f", results, check.want)
				}
				sum := check.spec.Base
				for _, contribution := range results[0].Contributions {
					sum += contribution.Points
				}
				if sum != check.want {
					t.Fatalf("breakdown=%v score=%d", results[0].Contributions, check.want)
				}
			}
		})
	}
}

func TestGoComplexityMethodNamesAndIndependentScopes(t *testing.T) {
	source := "package demo\ntype T struct{}\nfunc (T) M() { if true {} }\nfunc f(){ for {} }\n"
	doc, err := parser.NewParser().Parse("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	for _, build := range []func() (MetricSpec, error){GoCyclomaticMetricSpec, GoCognitiveMetricSpec} {
		spec, err := build()
		if err != nil {
			t.Fatal(err)
		}
		results, err := AnalyzeMetrics(context.Background(), spec, doc, EvaluateOptions{})
		if err != nil || len(results) != 2 || results[0].Name != "M" || results[1].Name != "f" {
			t.Fatalf("scores=%v error=%v", results, err)
		}
	}
}

func TestMetricsValidateAndFailClosed(t *testing.T) {
	spec, err := GoCyclomaticMetricSpec()
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	spec.Rules[0].Points = -1
	if err := spec.Validate(); err == nil {
		t.Fatal("accepted negative rule weight")
	}
	spec, err = GoCognitiveMetricSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.NameField = "not_a_go_grammar_field"
	if err := spec.Validate(); err == nil {
		t.Fatal("accepted unknown scope field without scanning source")
	}
	spec, err = GoCognitiveMetricSpec()
	if err != nil {
		t.Fatal(err)
	}
	spec.Rules[0].FlatAlternativeField = "not_a_go_grammar_field"
	if err := spec.Validate(); err == nil {
		t.Fatal("accepted unknown alternative field")
	}
	spec, err = GoCyclomaticMetricSpec()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, source string
		ctx          context.Context
		options      EvaluateOptions
		code         string
	}{
		{"parse", "package demo\nfunc f() { if { } }\n", context.Background(), EvaluateOptions{}, "SOURCE_PARSE"},
		{"cancel", "package demo\nfunc f() {}\n", cancelledMetricContext(), EvaluateOptions{}, "EVALUATION_CANCELLED"},
		{"limit", "package demo\nfunc f() {}\n", context.Background(), EvaluateOptions{MaxSteps: 1}, "LIMIT_AST_STEPS"},
		{"source-bytes", "package demo\nfunc f() {}\n", context.Background(), EvaluateOptions{MaxSourceBytes: 1}, "LIMIT_SOURCE_BYTES"},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := parser.NewParser().Parse("go", test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer document.Close()
			results, err := AnalyzeMetrics(test.ctx, spec, document, test.options)
			var evaluationError *EvaluationError
			if !errors.As(err, &evaluationError) || evaluationError.Code != test.code || len(results) != 0 {
				t.Fatalf("results=%v error=%v want=%s", results, err, test.code)
			}
		})
	}
	bad, err := parser.NewParser().Parse("typescript", "const a=1;\n")
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if results, err := AnalyzeMetrics(context.Background(), spec, bad, EvaluateOptions{}); err == nil || len(results) != 0 || !strings.Contains(err.Error(), "languages differ") {
		t.Fatalf("wrong language: %v %v", results, err)
	}
}

func cancelledMetricContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestMetricSelectorsWorkAcrossSupportedLanguages(t *testing.T) {
	cases := []struct{ language, source string }{
		{"c", "int value = 1;\n"}, {"cpp", "int value = 1;\n"},
		{"csharp", "class App { int value = 1; }\n"},
		{"dart", "void run() { final value = 1; }\n"},
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
	seen := map[string]bool{}
	for _, test := range cases {
		t.Run(test.language, func(t *testing.T) {
			seen[test.language] = true
			doc, err := parser.NewParser().Parse(test.language, test.source)
			if err != nil {
				t.Fatal(err)
			}
			defer doc.Close()
			root := doc.Root()
			child := root.NamedChild(0)
			if !child.Valid() {
				t.Fatal("no first named child")
			}
			query := fmt.Sprintf("language %s\n{metric: {scope: %s(), base: 1, above: 0, rules: [{id: \"child\", query: %s(), points: 1}]}}", test.language, root.Kind(), child.Kind())
			compiled, err := CompileMetric([]byte(query), CompileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			results, err := AnalyzeMetrics(context.Background(), compiled.Spec, doc, EvaluateOptions{})
			if err != nil || len(results) != 1 || results[0].Score < 2 {
				t.Fatalf("results=%v error=%v", results, err)
			}
		})
	}
	for _, language := range SupportedLanguages() {
		if !seen[language.ID] {
			t.Fatalf("no metric test for %q", language.ID)
		}
	}
}

func TestEvaluateProgramsSharesResultsAndCandidateNodes(t *testing.T) {
	doc, err := parser.NewParser().Parse("go", "package demo\nfunc f(x bool){if x {}}\n")
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	functions, err := Compile([]byte("language go\nfunction_declaration()"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ifs, err := Compile([]byte("language go\nif_statement()"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := EvaluatePrograms(context.Background(), []*Program{functions, ifs}, doc, EvaluateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, program := range []*Program{functions, ifs} {
		separate, err := Evaluate(context.Background(), program, doc, EvaluateOptions{})
		if err != nil || !reflect.DeepEqual(separate, rows[i]) {
			t.Fatalf("row=%d shared=%v separate=%v error=%v", i, rows[i], separate, err)
		}
		if len(rows[i]) != 1 {
			t.Fatalf("row=%d: matches=%v", i, rows[i])
		}
		kind, nodeRange, ok := rows[i][0].CandidateNode()
		if !ok || kind == "" || nodeRange.EndByte <= nodeRange.StartByte {
			t.Fatalf("candidate=%q %+v %v", kind, nodeRange, ok)
		}
	}
	if results, err := EvaluatePrograms(context.Background(), []*Program{functions, ifs}, doc, EvaluateOptions{MaxSteps: 1}); err == nil || len(results) != 0 {
		t.Fatalf("incomplete results=%v error=%v", results, err)
	}
}
