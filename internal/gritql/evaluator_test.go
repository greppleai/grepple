package gritql

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/parser"
)

func evaluateForTest(t *testing.T, pattern, source string, options EvaluateOptions) []EvaluationMatch {
	t.Helper()
	program, err := Compile([]byte("language go\n"+pattern), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	document, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(document.Close)
	matches, err := Evaluate(context.Background(), program, document, options)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestEvaluateSnippetWhereRegexAndOrderedConstraintBindings(t *testing.T) {
	t.Parallel()
	matches := evaluateForTest(t,
		"`f($x)` where { $x <: `g($y)`, $y <: r\"^alpha$\" }",
		"package p\nvar _ = f(g(alpha))\nvar _ = f(g(beta))\n", EvaluateOptions{})
	if len(matches) != 1 {
		t.Fatalf("matches=%d want 1: %+v", len(matches), matches)
	}
	bindings := matches[0].Bindings().All()
	if len(bindings) != 2 || bindings[0].Text() != "g(alpha)" || bindings[1].Text() != "alpha" {
		t.Fatalf("unexpected ordered bindings: %+v", bindings)
	}
	if got := matches[0].Range(); got.StartByte >= got.EndByte || sourceSlice("package p\nvar _ = f(g(alpha))\nvar _ = f(g(beta))\n", got) != "f(g(alpha))" {
		t.Fatalf("unexpected range: %+v", got)
	}
}

func TestEvaluateContainsIsReflexiveAndDeduplicates(t *testing.T) {
	t.Parallel()
	source := "package p\nfunc f(){ target() }\n"
	matches := evaluateForTest(t, "contains `target()`", source, EvaluateOptions{})
	if len(matches) != 1 || sourceSlice(source, matches[0].Range()) != "target()" {
		t.Fatalf("matches=%v", matches)
	}
}

func TestEvaluateWithinKeepsOriginalCandidateRange(t *testing.T) {
	t.Parallel()
	source := "package p\nvar _ = outer(inner)\n"
	matches := evaluateForTest(t, "within `outer($arg)`", source, EvaluateOptions{})
	seenInner := false
	for _, match := range matches {
		if sourceSlice(source, match.Range()) == "inner" {
			seenInner = true
			binding := match.Bindings().All()[0]
			if binding.Text() != "inner" {
				t.Fatalf("ancestor binding text=%q", binding.Text())
			}
		}
	}
	if !seenInner {
		t.Fatalf("within did not retain descendant range: %v", matches)
	}
}

func TestListContainsVisitsSelfDirectElementsAndNestedNodesPreorder(t *testing.T) {
	t.Parallel()
	source := "package p\nfunc f(){ h(a, g(b)) }\n"
	candidate, nodes := listCandidateForTest(t, source, "a, g(b)")
	options := normalizeEvaluateOptions(EvaluateOptions{})
	budget := &evaluationBudget{ctx: context.Background(), maxSteps: options.MaxSteps}
	gotCandidates := containmentCandidates(candidate, nodes, budget)
	if budget.err != nil {
		t.Fatal(budget.err)
	}

	got := make([]string, 0, len(gotCandidates))
	seen := make(map[string]bool)
	for i, nested := range gotCandidates {
		if i == 0 {
			got = append(got, "list:"+sourceSlice(source, nested.rng))
			continue
		}
		key := nodeRangeKey(nested.node.Kind(), nested.node.Range())
		if seen[key] {
			t.Fatalf("duplicate contained node %s", key)
		}
		seen[key] = true
		got = append(got, nested.node.Kind()+":"+nested.node.Text())
	}
	want := []string{
		"list:a, g(b)",
		"identifier:a",
		"call_expression:g(b)",
		"identifier:g",
		"argument_list:(b)",
		"identifier:b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("contained candidates=%q want %q", got, want)
	}
}

func TestListWithinVisitsSelfLowestCommonParentAndAncestorsNearestFirst(t *testing.T) {
	t.Parallel()
	source := "package p\nfunc f(){ h(a, g(b)) }\n"
	candidate, nodes := listCandidateForTest(t, source, "a, g(b)")
	options := normalizeEvaluateOptions(EvaluateOptions{})
	budget := &evaluationBudget{ctx: context.Background(), maxSteps: options.MaxSteps}
	gotCandidates := withinCandidates(candidate, nodes, budget)
	if budget.err != nil {
		t.Fatal(budget.err)
	}

	got := []string{"list"}
	for _, containing := range gotCandidates[1:] {
		got = append(got, containing.node.Kind())
	}
	want := []string{
		"list",
		"argument_list",
		"call_expression",
		"expression_statement",
		"statement_list",
		"block",
		"function_declaration",
		"source_file",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("within candidates=%q want %q", got, want)
	}
	if gotCandidates[1].node.Range() != candidate.parent.Range() {
		t.Fatalf("first container is not list lowest common parent")
	}
}

func listCandidateForTest(t *testing.T, source, text string) (evalCandidate, map[string]evalCandidate) {
	t.Helper()
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(doc.Close)
	options := normalizeEvaluateOptions(EvaluateOptions{})
	budget := &evaluationBudget{ctx: context.Background(), maxSteps: options.MaxSteps}
	candidates, nodes, err := collectEvaluationCandidates(doc.Root(), options, budget)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.sequence && sourceSlice(source, candidate.rng) == text {
			return candidate, nodes
		}
	}
	t.Fatalf("list candidate %q not found", text)
	return evalCandidate{}, nil
}

func TestEvaluateBooleanTransactionsMaybeAndDedupe(t *testing.T) {
	t.Parallel()
	source := "package p\nvar _ = f(a)\n"
	matches := evaluateForTest(t, "or { `f($x)`, and { `f($x)`, maybe `missing`, }, }", source, EvaluateOptions{})
	if len(matches) != 1 || matches[0].Bindings().Len() != 1 {
		t.Fatalf("duplicate branches were not collapsed: %v", matches)
	}

	matches = evaluateForTest(t, "and { `f($x)`, not `g($x)`, }", source, EvaluateOptions{})
	if len(matches) != 1 || matches[0].Bindings().All()[0].Text() != "a" {
		t.Fatalf("not transaction changed bindings: %v", matches)
	}
}

func TestEvaluateStatementSequenceCandidates(t *testing.T) {
	t.Parallel()
	source := "package p\nfunc f(){ before(); x(); y(); after() }\n"
	matches := evaluateForTest(t, "`x(); y()`", source, EvaluateOptions{})
	if len(matches) != 1 || sourceSlice(source, matches[0].Range()) != "x(); y()" {
		t.Fatalf("sequence matches=%v", matches)
	}
	semicolonMatches := evaluateForTest(t, "`x();`", source, EvaluateOptions{})
	if len(semicolonMatches) != 1 || sourceSlice(source, semicolonMatches[0].Range()) != "x();" {
		t.Fatalf("one-item explicit-semicolon sequence matches=%v", semicolonMatches)
	}
}

func TestTraversalEnumeratesEveryRepeatedGoPositionInCanonicalOrder(t *testing.T) {
	t.Parallel()
	source := "package p\ntype I interface { int | string }\ntype S struct { A, B int; C string }\nfunc f(a, b int, c string) { g(x, y,) }\n"
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	options := normalizeEvaluateOptions(EvaluateOptions{})
	budget := &evaluationBudget{ctx: context.Background(), maxSteps: options.MaxSteps}
	candidates, _, err := collectEvaluationCandidates(doc.Root(), options, budget)
	if err != nil {
		t.Fatal(err)
	}
	wantPositions := map[string]bool{
		"source_file:": false, "interface_type:": false, "type_elem:": false,
		"field_declaration_list:": false, "field_declaration:name": false,
		"parameter_list:": false, "parameter_declaration:name": false, "argument_list:": false,
	}
	var arguments []string
	for _, candidate := range candidates {
		if !candidate.sequence {
			continue
		}
		key := candidate.parent.Kind() + ":" + candidate.target.field
		if _, wanted := wantPositions[key]; wanted {
			wantPositions[key] = true
		}
		if candidate.parent.Kind() == "argument_list" {
			arguments = append(arguments, sourceSlice(source, candidate.rng))
		}
	}
	for position, found := range wantPositions {
		if !found {
			t.Errorf("missing traversal list position %s", position)
		}
	}
	wantArguments := []string{"x, y", "x", "y"}
	if !reflect.DeepEqual(arguments, wantArguments) {
		t.Fatalf("argument candidates=%q want %q", arguments, wantArguments)
	}
}

func TestTopLevelNotAndMaybeUseListCandidatesWithoutSeparatorRanges(t *testing.T) {
	t.Parallel()
	source := "package p\nvar _ = f(a, b,)\n"
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	options := normalizeEvaluateOptions(EvaluateOptions{})
	budget := &evaluationBudget{ctx: context.Background(), maxSteps: options.MaxSteps}
	candidates, _, err := collectEvaluationCandidates(doc.Root(), options, budget)
	if err != nil {
		t.Fatal(err)
	}
	uniqueRanges := make(map[[2]int]bool)
	argumentLists := 0
	for _, candidate := range candidates {
		uniqueRanges[[2]int{candidate.rng.StartByte, candidate.rng.EndByte}] = true
		if candidate.sequence && candidate.parent.Kind() == "argument_list" {
			argumentLists++
			if text := sourceSlice(source, candidate.rng); text == "a, b," || strings.HasSuffix(text, ",") {
				t.Fatalf("list candidate retained trailing separator: %q", text)
			}
		}
	}
	if argumentLists != 3 {
		t.Fatalf("argument list candidate count=%d want 3", argumentLists)
	}
	for _, pattern := range []string{"not `absent`", "maybe `absent`"} {
		matches := evaluateForTest(t, pattern, source, EvaluateOptions{})
		if len(matches) != len(uniqueRanges) {
			t.Fatalf("%s emitted %d ranges want %d traversal ranges", pattern, len(matches), len(uniqueRanges))
		}
	}
}

func TestEvaluateLimitsCancellationDeadlineAndParseFailureAreTyped(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language go\n`x`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := parser.ParseDocument("go", "package p\nvar _ = x\n")
	if err != nil {
		t.Fatal(err)
	}
	defer valid.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Evaluate(ctx, program, valid, EvaluateOptions{})
	assertEvaluationCode(t, err, "EVALUATION_CANCELLED")
	_, err = Evaluate(context.Background(), program, valid, EvaluateOptions{MaxCandidates: 1})
	assertEvaluationCode(t, err, "LIMIT_CANDIDATES")
	_, err = Evaluate(context.Background(), program, valid, EvaluateOptions{Deadline: time.Now().Add(-time.Second)})
	assertEvaluationCode(t, err, "LIMIT_TIME_FILE")
	_, err = Evaluate(context.Background(), program, valid, EvaluateOptions{MaxSteps: 1})
	assertEvaluationCode(t, err, "LIMIT_AST_STEPS")

	invalid, err := parser.ParseDocument("go", "package p\nfunc f(\n")
	if err != nil {
		t.Fatal(err)
	}
	defer invalid.Close()
	_, err = Evaluate(context.Background(), program, invalid, EvaluateOptions{})
	assertEvaluationCode(t, err, "SOURCE_PARSE")
}

func assertEvaluationCode(t *testing.T, err error, code string) {
	t.Helper()
	var evaluationErr *EvaluationError
	if !errors.As(err, &evaluationErr) || evaluationErr.Code != code {
		t.Fatalf("error=%v want EvaluationError code %s", err, code)
	}
}

func sourceSlice(source string, r parser.Range) string {
	if r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > len(source) {
		return ""
	}
	return source[r.StartByte:r.EndByte]
}
