package gritql

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCompileEveryV1OperatorSnapshot(t *testing.T) {
	t.Parallel()
	const source = "language go\nand { contains `f($x, \\`q\\`)`, or { not `a`, maybe within `b`, }, } where { $x <: r\"^x\\n\\x41\\u0042$\", }"
	program, err := Compile([]byte(source), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if program.Compatibility() != Compatibility || program.Language() != "go" || program.Range().EndByte != len(source) {
		t.Fatalf("program envelope = %#v", program)
	}

	root := program.Root()
	if root.Kind() != KindWhere || len(root.Constraints()) != 1 {
		t.Fatalf("root = %s constraints=%d", root.Kind(), len(root.Constraints()))
	}
	and := root.Children()[0]
	if and.Kind() != KindAnd {
		t.Fatalf("prefix = %s", and.Kind())
	}
	contains := and.Children()[0]
	gotSnippet := contains.Children()[0].Text()
	if contains.Kind() != KindContains || gotSnippet != "f($x, `q`)" {
		t.Fatalf("decoded snippet = %q", gotSnippet)
	}
	or := and.Children()[1]
	if got := []Kind{or.Children()[0].Kind(), or.Children()[1].Kind(), or.Children()[1].Children()[0].Kind()}; !reflect.DeepEqual(got, []Kind{KindNot, KindMaybe, KindWithin}) {
		t.Fatalf("nested operators = %v", got)
	}
	constraint := root.Constraints()[0]
	if constraint.LHS().Name != "$x" || constraint.RHS().Kind() != KindRegex || constraint.RHS().Text() != "^x\nAB$" || !constraint.RHS().MatchRegex("x\nAB") {
		t.Fatalf("constraint snapshot: lhs=%+v rhs=%q", constraint.LHS(), constraint.RHS().Text())
	}
	want := FeatureSnippet | FeatureRegex | FeatureAnd | FeatureOr | FeatureNot | FeatureMaybe | FeatureContains | FeatureWithin | FeatureWhere | FeatureVariables
	if program.Features() != want {
		t.Fatalf("features = %b, want %b", program.Features(), want)
	}
}

func TestCompileVariablesDeterministicAndAnonymous(t *testing.T) {
	t.Parallel()
	const source = "language go\nor { `f($b, $_)`, `g($a, $b)`, } where { $a <: `h($c)`, $c <: `z`, }"
	for range 20 {
		program, err := Compile([]byte(source), CompileOptions{})
		if err != nil {
			t.Fatal(err)
		}
		vars := program.Variables()
		if got := []string{vars[0].Name, vars[1].Name, vars[2].Name}; !reflect.DeepEqual(got, []string{"$b", "$a", "$c"}) {
			t.Fatalf("variables = %v", got)
		}
		if vars[0].ID != 1 || vars[1].ID != 2 || vars[2].ID != 3 {
			t.Fatalf("IDs = %+v", vars)
		}
		refs := program.Root().Children()[0].Children()[0].Variables()
		if len(refs) != 2 || refs[1].ID != 0 || !refs[1].Anonymous {
			t.Fatalf("anonymous refs = %+v", refs)
		}
	}
}

func TestCompileNestedWhereAndOrderedConstraintScope(t *testing.T) {
	t.Parallel()
	valid := "language go\n`f($x)` where { $x <: `g($y)` where { $y <: r\"ok\" }, $y <: `z`, }"
	program, err := Compile([]byte(valid), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	constraints := program.Root().Constraints()
	if len(constraints) != 2 || constraints[0].LHS().Name != "$x" || constraints[1].LHS().Name != "$y" {
		t.Fatalf("constraint order = %+v", constraints)
	}
	if declaration := program.Declaration(); declaration.Name != "go" || declaration.Range.StartByte != 0 || declaration.Range.EndByte != len("language go") {
		t.Fatalf("language declaration = %+v", declaration)
	}
	// Boolean siblings are isolated transactions; one sibling cannot make a
	// binding available to another sibling's where prefix.
	branchScope := "language go\nand { `f($x)`, `g` where { $x <: `z` }, }"
	_, err = Compile([]byte(branchScope), CompileOptions{})
	assertCompileCode(t, err, "PATTERN_INVALID_CONTEXT")

	for _, source := range []string{
		"language go\n`x` where { $missing <: `x` }",
		"language go\n`x` where { $later <: `f($later)`, }",
	} {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_INVALID_CONTEXT")
	}
}

func TestCompileRegexContextValidityAndLimits(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"language go\nr\"x\"",
		"language go\nnot r\"x\"",
		"language go\n`f($x)` where { $x <: and { r\"x\", `x`, } }",
	} {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_INVALID_CONTEXT")
	}
	_, err := Compile([]byte("language go\n`f($x)` where { $x <: r\"[\" }"), CompileOptions{})
	assertCompileCode(t, err, "PATTERN_INVALID_REGEX")
	_, err = Compile([]byte("language go\n`f($x)` where { $x <: r\"abcd\" }"), CompileOptions{MaxRegexBytes: 3})
	assertCompileCode(t, err, "PATTERN_INVALID_REGEX")
}

func TestCompileStableErrorsAndUnsupportedSection8(t *testing.T) {
	t.Parallel()
	unsupported := []string{
		"language go\n`x` => `y`",              // rewrite
		"language go\npattern foo() { `x` }",   // definition/call
		"language go\nimport \"lib\"",          // import
		"language go\nmodule foo",              // module/library envelope
		"language go\nmultifile { `x`, `y` }",  // multifile
		"language go\nsequential { `x`, `y` }", // sequential
		"language go\nCall(name=`x`)",          // AST constructor
		"language go\ngo`x`",                   // language qualifier
		"language go\n`x` as $x",               // as capture
		"language go\n$x = foo()",              // assignment/function
		"language go\n`x` where { $x == `x` }", // equality
		"language go /* block comment */\n`x`", // absent comment form
		"language ruby\n`x`",                   // unsupported target
	}
	for _, source := range unsupported {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_UNSUPPORTED")
		var ce *CompileError
		errors.As(err, &ce)
		if ce.Class != "unsupported" || ce.Range.StartByte != 0 || ce.Range.EndByte != len(source) || ce.Range.Start.Line != 1 || ce.Range.Start.Column != 1 {
			t.Errorf("unstable unsupported diagnostic for %q: %+v", source, ce)
		}
	}
	for _, source := range []string{"`x`", "language go\n", "language go\nlanguage go\n`x`"} {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_PARSE")
		var ce *CompileError
		if !errors.As(err, &ce) || errors.Unwrap(ce) == nil || ce.Range.EndByte != len(source) {
			t.Fatalf("malformed passthrough = %#v", err)
		}
	}
}

func TestCompilePossibleBindingTransactions(t *testing.T) {
	t.Parallel()
	rejected := []string{
		"language go\nand { `f($x)`, `g` where { $x <: `z` }, }",
		"language go\nand { `g` where { $x <: `z` }, `f($x)`, }",
		"language go\n`g` where { $x <: `z`, $x <: `f($x)`, }",
		"language go\nand { not `f($x)`, `g`, } where { $x <: `z` }",
	}
	for _, source := range rejected {
		_, err := Compile([]byte(source), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_INVALID_CONTEXT")
	}
	accepted := []string{
		"language go\nand { `f($x)`, `g($y)`, } where { $x <: `z` }",
		"language go\nmaybe `f($x)` where { $x <: `z` }",
		"language go\n`f($x)` where { $x <: `g($y)` where { $y <: `h($z)` }, $z <: `ok`, }",
	}
	for _, source := range accepted {
		if _, err := Compile([]byte(source), CompileOptions{}); err != nil {
			t.Errorf("Compile(%q): %v", source, err)
		}
	}
}

func TestCompileResourceLimitsAndClamping(t *testing.T) {
	t.Parallel()
	depthSource := "language go\ncontains contains contains `x`"
	_, err := Compile([]byte(depthSource), CompileOptions{MaxDepth: 2})
	assertCompileCode(t, err, "LIMIT_PARSE_DEPTH")
	var depthErr *CompileError
	if !errors.As(err, &depthErr) || depthErr.Class != "resource" || depthErr.Range != nil {
		t.Fatalf("depth diagnostic = %#v", err)
	}

	patternSource := "language go\n`x`"
	_, err = Compile([]byte(patternSource), CompileOptions{MaxPatternBytes: len(patternSource) - 1})
	assertCompileCode(t, err, "LIMIT_PATTERN_BYTES")
	var sizeErr *CompileError
	if !errors.As(err, &sizeErr) || sizeErr.Range == nil || sizeErr.Range.EndByte != len(patternSource) {
		t.Fatalf("pattern-size diagnostic = %#v", err)
	}

	hardDepth := "language go\n" + strings.Repeat("not ", hardMaxDepth+1) + "`x`"
	_, err = Compile([]byte(hardDepth), CompileOptions{MaxDepth: hardMaxDepth * 2})
	assertCompileCode(t, err, "LIMIT_PARSE_DEPTH")

	oversized := bytes.Repeat([]byte{'x'}, hardMaxPatternBytes+1)
	oversized[0], oversized[1] = 0xff, '\n'
	_, err = Compile(oversized, CompileOptions{MaxPatternBytes: hardMaxPatternBytes * 2})
	assertCompileCode(t, err, "LIMIT_PATTERN_BYTES")
	if !errors.As(err, &sizeErr) || sizeErr.Range == nil || sizeErr.Range.EndByte != len(oversized) || sizeErr.Range.End.Line != 2 || sizeErr.Range.End.Column != len(oversized)-1 {
		t.Fatalf("hard pattern clamp/constant-memory range = %#v", err)
	}

	largeRegex := strings.Repeat("a", hardMaxRegexBytes+1)
	regexSource := "language go\n`f($x)` where { $x <: r\"" + largeRegex + "\" }"
	_, err = Compile([]byte(regexSource), CompileOptions{MaxRegexBytes: hardMaxRegexBytes * 2})
	assertCompileCode(t, err, "PATTERN_INVALID_REGEX")

	normalized := normalizedOptions(CompileOptions{
		MaxPatternBytes: hardMaxPatternBytes + 1, MaxRegexBytes: hardMaxRegexBytes + 1,
		MaxRegexInstructions: hardMaxRegexInstructions + 1, MaxDepth: hardMaxDepth + 1,
	})
	if normalized.MaxPatternBytes != hardMaxPatternBytes || normalized.MaxRegexBytes != hardMaxRegexBytes || normalized.MaxRegexInstructions != hardMaxRegexInstructions || normalized.MaxDepth != hardMaxDepth {
		t.Fatalf("hard clamps not applied: %+v", normalized)
	}
}

func TestCompileRecognizedUnsupportedNamedNodes(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"includes `x`", "after `x`", "before `x`", "orelse { `x`, `y` }",
		"any { `x`, `y` }", "if ($x == `x`) { `y` } else { `z` }",
		"`x` limit 1", "bubble($x) `x`", "some `x`", "every `x`",
		"like { `x` }", "[1, 2]", "{x: 1}", "\"x\"", "1", "Top", "Bottom",
	} {
		_, err := Compile([]byte("language go\n"+body), CompileOptions{})
		assertCompileCode(t, err, "PATTERN_UNSUPPORTED")
	}
	_, err := Compile([]byte("language go\nfrobnicate"), CompileOptions{})
	assertCompileCode(t, err, "PATTERN_PARSE")
}

func TestProgramImmutableConcurrentReadAndCompile(t *testing.T) {
	t.Parallel()
	const source = "language go\n`f($x)` where { $x <: r\"x\" }"
	program, err := Compile([]byte(source), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	variables := program.Variables()
	variables[0].Name = "mutated"
	children := program.Root().Children()
	children[0] = Expression{}
	refs := program.Root().Children()[0].Variables()
	refs[0].Name = "mutated"
	if program.Variables()[0].Name != "$x" || program.Root().Children()[0].Variables()[0].Name != "$x" {
		t.Fatal("accessor mutated program")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				p, e := Compile([]byte(source), CompileOptions{})
				if e != nil || p.Root().Constraints()[0].RHS().Text() != "x" || !program.Root().Constraints()[0].RHS().MatchRegex("x") {
					t.Errorf("concurrent compile/read failed: %v", e)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func assertCompileCode(t *testing.T, err error, code string) {
	t.Helper()
	var compileErr *CompileError
	if !errors.As(err, &compileErr) || compileErr.Code != code {
		t.Fatalf("error = %#v, want %s", err, code)
	}
}
