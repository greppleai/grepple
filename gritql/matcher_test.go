package gritql

import (
	"reflect"
	"sync"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func parseGoForMatch(t *testing.T, source string) *parser.Document {
	t.Helper()
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(doc.Close)
	if diagnostics := doc.ParseDiagnostics(); len(diagnostics) != 0 {
		t.Fatalf("source diagnostics: %+v", diagnostics)
	}
	return doc
}

func findMatchNode(t *testing.T, root parser.Node, kind, text string) parser.Node {
	t.Helper()
	stack := []parser.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Kind() == kind && (text == "" || n.Text() == text) {
			return n
		}
		children := n.Children()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	t.Fatalf("node %s %q not found", kind, text)
	return parser.Node{}
}

func TestMatchTemplateStructuralTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, snippet, source, kind, text string
		want                              bool
	}{
		{"exact", "a + b", "package p\nvar _ = a + b\n", "binary_expression", "a + b", true},
		{"literal leaf differs", "a + b", "package p\nvar _ = a + c\n", "binary_expression", "a + c", false},
		{"partial rejected", "a + b", "package p\nvar _ = a + b + c\n", "binary_expression", "a + b + c", false},
		{"nested", "T{K: f(x)}", "package p\nvar _ = T{ K: f(x) }\n", "composite_literal", "T{ K: f(x) }", true},
		{"operator differs", "a + b", "package p\nvar _ = a - b\n", "binary_expression", "a - b", false},
		{"literal spelling", "0x10", "package p\nvar _ = 16\n", "int_literal", "16", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpl := compileSnippetForTest(t, tc.snippet).Template()
			doc := parseGoForMatch(t, tc.source)
			n := findMatchNode(t, doc.Root(), tc.kind, tc.text)
			result, got := MatchNode(tmpl, n)
			if got != tc.want {
				t.Fatalf("matched=%v want=%v", got, tc.want)
			}
			if got && result.Range().StartByte >= result.Range().EndByte {
				t.Fatalf("bad range: %+v", result.Range())
			}
		})
	}
}

func TestMatchTemplateRepeatedNormalizationAndRollback(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "$x == $x").Template()
	cases := []struct {
		source string
		want   bool
	}{
		{"package p\nvar _ = f(a /* left */) == f(a)\n", true},
		{"package p\nvar _ = f(a) == f(b)\n", false},
		{"package p\nvar _ = func(){ x(); y() } == func(){ x()\ny() }\n", false}, // explicit semicolon is significant
		{"package p\nvar _ = func(){ x()\ny() } == func(){ x()\ny() }\n", true},
	}
	for _, tc := range cases {
		doc := parseGoForMatch(t, tc.source)
		n := findMatchNode(t, doc.Root(), "binary_expression", "")
		got, ok := MatchNode(tmpl, n)
		if ok != tc.want {
			t.Errorf("%q matched=%v want=%v", tc.source, ok, tc.want)
		}
		if !ok && got.Bindings().Len() != 0 {
			t.Fatal("failed attempt leaked a binding")
		}
	}
}

func TestMatchTemplateRepeatedLists(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "f($args) == f($args)").Template()
	cases := []struct {
		expression string
		want       bool
	}{
		{"f(a, /* trivia */ b) == f(a,b)", true},
		{"f(a,b) == f(a,c)", false},
		{"f(a,b) == f(a)", false},
		{"f() == f()", true},
		{"f(a,) == f(a)", true},
		{"f(a,) == f(a,)", true},
	}
	for _, tc := range cases {
		doc := parseGoForMatch(t, "package p\nvar _ = "+tc.expression+"\n")
		n := findMatchNode(t, doc.Root(), "binary_expression", tc.expression)
		result, ok := MatchNode(tmpl, n)
		if ok != tc.want {
			t.Errorf("%q matched=%v want=%v", tc.expression, ok, tc.want)
		}
		if !ok && result.Bindings().Len() != 0 {
			t.Fatal("failed list attempt leaked bindings")
		}
	}
}

func TestMatchTemplateRepeatedTypeUnion(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "interface { $types | string }").Template()
	doc := parseGoForMatch(t, "package p\ntype C interface { int | bool | string }\n")
	union := findMatchNode(t, doc.Root(), "interface_type", "interface { int | bool | string }")
	result, ok := MatchNode(tmpl, union)
	if !ok {
		t.Fatal("type slot did not consume the longer union prefix")
	}
	binding := result.Bindings().All()[0]
	if binding.Text() != "int | bool" || len(binding.Ranges()) != 2 || len(binding.Structural()) != len(binding.Ranges()) {
		t.Fatalf("binding text=%q ranges=%d structural=%d", binding.Text(), len(binding.Ranges()), len(binding.Structural()))
	}
	for _, node := range binding.Structural() {
		if node.Kind() == "|" {
			t.Fatal("public structural elements exposed a separator")
		}
	}

	repeated := compileSnippetForTest(t, "interface { $types | string; $types | int }").Template()
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"bool | rune | string; bool | rune | int", true},
		{"bool | rune | string; bool | byte | int", false},
	} {
		doc := parseGoForMatch(t, "package p\ntype C interface { "+tc.body+" }\n")
		n := findMatchNode(t, doc.Root(), "interface_type", "interface { "+tc.body+" }")
		if _, got := MatchNode(repeated, n); got != tc.want {
			t.Errorf("%q matched=%v want=%v", tc.body, got, tc.want)
		}
	}
}

func TestMatchTemplateSlotManyArguments(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "exec.Command($args)").Template()
	cases := []struct {
		call        string
		count       int
		bindingText string
	}{
		{"exec.Command()", 0, ""},
		{"exec.Command(name)", 1, "name"},
		{"exec.Command(name,)", 1, "name"},
		{"exec.Command(name, \"a\", env.Value())", 3, "name, \"a\", env.Value()"},
	}
	for _, tc := range cases {
		doc := parseGoForMatch(t, "package p\nvar _ = "+tc.call+"\n")
		n := findMatchNode(t, doc.Root(), "call_expression", tc.call)
		got, ok := MatchNode(tmpl, n)
		if !ok {
			t.Fatalf("did not match %q", tc.call)
		}
		bindings := got.Bindings().All()
		if len(bindings) != 1 || bindings[0].Kind() != BindingList || len(bindings[0].Ranges()) != tc.count || bindings[0].Text() != tc.bindingText {
			t.Fatalf("%q binding=%+v ranges=%d text=%q", tc.call, bindings, len(bindings[0].Ranges()), bindings[0].Text())
		}
		if tc.count == 0 && bindings[0].Range().StartByte != bindings[0].Range().EndByte {
			t.Fatalf("empty range is not zero width: %+v", bindings[0].Range())
		}
	}
}

func TestMatchTemplateAbsentStatementListBindsEmpty(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "if ok { $stmt }").Template()
	for _, tc := range []struct {
		name, body, bindingText string
		count                   int
	}{
		{name: "empty", body: "", bindingText: "", count: 0},
		{name: "nonempty nested block", body: "{ nested() }", bindingText: "{ nested() }", count: 1},
		{name: "nonempty nested if", body: "if inner { work() }", bindingText: "if inner { work() }", count: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertStatementListBinding(t, tmpl, tc.body, tc.bindingText, tc.count)
		})
	}
}

func assertStatementListBinding(t *testing.T, tmpl Template, body, bindingText string, count int) {
	t.Helper()
	source := "package p\nfunc f(){ if ok { " + body + " } }\n"
	doc := parseGoForMatch(t, source)
	statement := findMatchNode(t, doc.Root(), "if_statement", "if ok { "+body+" }")
	result, ok := MatchNode(tmpl, statement)
	if !ok {
		t.Fatalf("template did not match %q", statement.Text())
	}
	binding := result.Bindings().All()[0]
	if binding.Kind() != BindingList || len(binding.Ranges()) != count || binding.Text() != bindingText {
		t.Fatalf("binding kind=%v ranges=%d text=%q", binding.Kind(), len(binding.Ranges()), binding.Text())
	}
	if count != 0 {
		return
	}
	block := findMatchNode(t, statement, "block", "{  }")
	closing := block.Children()[len(block.Children())-1].Range()
	want := zeroRange(closing, true)
	if binding.Range() != want {
		t.Fatalf("empty binding range=%+v want closing-brace anchor=%+v", binding.Range(), want)
	}
}

func TestMatchTemplateAbsentExpressionListBindsEmpty(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "return $values").Template()
	doc := parseGoForMatch(t, "package p\nfunc f(){ return }\n")
	statement := findMatchNode(t, doc.Root(), "return_statement", "return")
	result, ok := MatchNode(tmpl, statement)
	if !ok {
		t.Fatal("optional expression_list absence did not match")
	}
	binding := result.Bindings().All()[0]
	want := zeroRange(statement.Children()[0].Range(), false)
	if binding.Kind() != BindingList || len(binding.Ranges()) != 0 || binding.Text() != "" || binding.Range() != want {
		t.Fatalf("empty expression-list binding range=%+v ranges=%d text=%q want=%+v", binding.Range(), len(binding.Ranges()), binding.Text(), want)
	}
}

func TestMatchTemplateRepeatedAbsentListsStayConsistent(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "func(){ if first { $stmt }; if second { $stmt } }").Template()
	for _, tc := range []struct {
		body string
		want bool
	}{
		{"func(){ if first {}; if second {} }", true},
		{"func(){ if first { work() }; if second { work() } }", true},
		{"func(){ if first {}; if second { work() } }", false},
		{"func(){ if first { work() }; if second {} }", false},
	} {
		doc := parseGoForMatch(t, "package p\nvar _ = "+tc.body+"\n")
		n := findMatchNode(t, doc.Root(), "func_literal", tc.body)
		result, got := MatchNode(tmpl, n)
		if got != tc.want {
			t.Errorf("%q matched=%v want=%v", tc.body, got, tc.want)
		}
		if !got && result.Bindings().Len() != 0 {
			t.Fatal("failed repeated empty-list match leaked bindings")
		}
	}
}

func TestMatchTemplateDoesNotSkipLiteralListContainers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		snippet, source, kind, text string
	}{
		{"if ok { work() }", "package p\nfunc f(){ if ok {} }\n", "if_statement", "if ok {}"},
		{"if ok { work(); $rest }", "package p\nfunc f(){ if ok {} }\n", "if_statement", "if ok {}"},
		{"return value", "package p\nfunc f(){ return }\n", "return_statement", "return"},
	} {
		doc := parseGoForMatch(t, tc.source)
		n := findMatchNode(t, doc.Root(), tc.kind, tc.text)
		if _, ok := MatchNode(compileSnippetForTest(t, tc.snippet).Template(), n); ok {
			t.Errorf("literal-bearing list template %q matched absent container", tc.snippet)
		}
	}
}

func TestMatchTemplateStatementAndDeclarationSequences(t *testing.T) {
	t.Parallel()
	cases := []struct {
		snippet, source, parentKind, sequenceKind string
	}{
		{"x++; y++", "package p\nfunc f(){ x++; y++ }\n", "statement_list", "statement_sequence"},
		{"x()\ny()", "package p\nfunc f(){ x()\n// trivia\ny() }\n", "statement_list", "statement_sequence"},
		{"var x int; var y int", "package p\nvar x int; var y int\n", "source_file", "declaration_sequence"},
	}
	for _, tc := range cases {
		doc := parseGoForMatch(t, tc.source)
		parent := findMatchNode(t, doc.Root(), tc.parentKind, "")
		children := parent.Children()
		start := 0
		if tc.parentKind == "source_file" {
			for start < len(children) && children[start].Kind() != "package_clause" {
				start++
			}
			if start < len(children) {
				start++
			}
		}
		if _, ok := MatchTemplate(compileSnippetForTest(t, tc.snippet).Template(), SequenceTarget(tc.sequenceKind, parent, start, len(children))); !ok {
			t.Errorf("sequence %q did not match", tc.snippet)
		}
	}
}

func TestMatchTemplateRootSlotRangesAndIncomingBindings(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "$x").Template()
	doc := parseGoForMatch(t, "package p\nvar _ = alpha\nvar _ = beta\n")
	alpha := findMatchNode(t, doc.Root(), "identifier", "alpha")
	beta := findMatchNode(t, doc.Root(), "identifier", "beta")
	first, ok := MatchNode(tmpl, alpha)
	if !ok || first.Range() != alpha.Range() || first.Bindings().Len() != 1 {
		t.Fatalf("root slot result=%+v ok=%v", first, ok)
	}
	if _, ok := MatchTemplateWithBindings(tmpl, NodeTarget(beta), first.Bindings()); ok {
		t.Fatal("incompatible incoming repeated binding matched")
	}
	bound := first.Bindings().All()[0]
	if bound.Text() != "alpha" || len(bound.Structural()) != 1 || bound.Structural()[0].Kind() != "identifier" || bound.Structural()[0].Lexeme() != "alpha" {
		t.Fatalf("unexpected root binding: text=%q structural=%+v", bound.Text(), bound.Structural())
	}
}

func TestMatchTemplateRootSlotManyEmptyAndMultiple(t *testing.T) {
	t.Parallel()
	ref := VariableRef{ID: 99, Name: "$items"}
	tmpl := Template{context: SnippetContextStatementList, rootSlot: &templateSlot{ref: ref, cardinality: SlotMany}}
	emptyDoc := parseGoForMatch(t, "package p\nfunc f(){ x() }\n")
	emptyParent := findMatchNode(t, emptyDoc.Root(), "statement_list", "")
	anchor := zeroRange(emptyParent.Children()[0].Range(), true)
	empty, ok := MatchTemplate(tmpl, SequenceTarget("statement_sequence", emptyParent, 0, 0))
	if !ok {
		t.Fatal("empty root list did not match")
	}
	binding, exists := empty.Bindings().Lookup(ref.ID)
	if !exists || binding.Kind() != BindingList || binding.Range() != anchor || len(binding.Ranges()) != 0 {
		t.Fatalf("empty binding=%+v exists=%v", binding, exists)
	}

	doc := parseGoForMatch(t, "package p\nfunc f(){ x(); y() }\n")
	list := findMatchNode(t, doc.Root(), "statement_list", "")
	multiple, ok := MatchTemplate(tmpl, SequenceTarget("statement_sequence", list, 0, len(list.Children())))
	if !ok || len(multiple.Bindings().All()[0].Ranges()) != 2 || multiple.Bindings().All()[0].Text() != "x(); y()" {
		t.Fatalf("multiple root list=%+v ok=%v", multiple, ok)
	}
}

func TestMatchTemplateEmptySequenceAnchorSkipsTriviaAndAutomaticSemicolons(t *testing.T) {
	t.Parallel()
	ref := VariableRef{ID: 100, Name: "$items"}
	tmpl := Template{context: SnippetContextStatementList, rootSlot: &templateSlot{ref: ref, cardinality: SlotMany}}
	doc := parseGoForMatch(t, "package p\nfunc f(){ x()\n// trivia\ny()\n}\n")
	list := findMatchNode(t, doc.Root(), "statement_list", "")
	children := list.Children()
	comment := -1
	for i, child := range children {
		if child.Kind() == "comment" {
			comment = i
			break
		}
	}
	if comment < 0 {
		t.Fatal("comment child not found")
	}
	x := findMatchNode(t, list, "expression_statement", "x()")
	want := zeroRange(x.Range(), false)
	result, ok := MatchTemplate(tmpl, SequenceTarget("statement_sequence", list, comment, comment))
	if !ok {
		t.Fatal("empty sequence did not match")
	}
	binding := result.Bindings().All()[0]
	if binding.Range() != want || result.Range() != want {
		t.Fatalf("anchor=%+v result=%+v want nearest significant=%+v", binding.Range(), result.Range(), want)
	}
}

func TestMatchTemplateImmutableConcurrent(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "f($args)").Template()
	doc := parseGoForMatch(t, "package p\nvar _ = f(a,b)\n")
	n := findMatchNode(t, doc.Root(), "call_expression", "f(a,b)")
	want, ok := MatchNode(tmpl, n)
	if !ok {
		t.Fatal("baseline did not match")
	}
	before := want.Bindings().All()[0].Ranges()
	copyRanges := want.Bindings().All()[0].Ranges()
	copyRanges[0] = parser.Range{}
	if reflect.DeepEqual(copyRanges, want.Bindings().All()[0].Ranges()) {
		t.Fatal("range accessor exposed mutable state")
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				got, matched := MatchNode(tmpl, n)
				if !matched || !reflect.DeepEqual(got.Bindings().All()[0].Ranges(), before) {
					t.Errorf("non-deterministic concurrent match")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestMatchTemplateRejectsRecoveryTree(t *testing.T) {
	t.Parallel()
	doc, err := parser.ParseDocument("go", "package p\nvar x = f(\n")
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if len(doc.ParseDiagnostics()) == 0 {
		t.Fatal("expected parse diagnostics")
	}
	if _, ok := MatchNode(compileSnippetForTest(t, "$x").Template(), doc.Root()); ok {
		t.Fatal("matched a recovery tree")
	}
}

func TestMatchTemplateRootSlotEnforcesGrammarCategory(t *testing.T) {
	t.Parallel()
	doc := parseGoForMatch(t, "package p\nvar value []int = call()\nconst answer = 42\ntype Value int\nfunc f(){ call() }\n")
	expression := compileSnippetForTest(t, "$x").Template()
	for _, tc := range []struct {
		kind, text string
		want       bool
	}{
		{"call_expression", "call()", true},
		{"expression_statement", "call()", false},
		{"var_declaration", "var value []int = call()", false},
		{"source_file", "", false},
	} {
		n := findMatchNode(t, doc.Root(), tc.kind, tc.text)
		if _, got := MatchNode(expression, n); got != tc.want {
			t.Errorf("expression slot against %s matched=%v want=%v", tc.kind, got, tc.want)
		}
	}
	rootSlot := func(context SnippetContext) Template {
		return Template{context: context, rootSlot: &templateSlot{ref: VariableRef{Anonymous: true}, cardinality: SlotOne}}
	}
	for _, tc := range []struct {
		context    SnippetContext
		kind, text string
		want       bool
	}{
		{SnippetContextType, "slice_type", "[]int", true},
		{SnippetContextType, "call_expression", "call()", false},
		{SnippetContextStatement, "expression_statement", "call()", true},
		{SnippetContextStatement, "var_declaration", "var value []int = call()", true},
		{SnippetContextStatement, "const_declaration", "const answer = 42", true},
		{SnippetContextStatement, "type_declaration", "type Value int", true},
		{SnippetContextDeclaration, "var_declaration", "var value []int = call()", true},
		{SnippetContextDeclaration, "expression_statement", "call()", false},
		{SnippetContextFile, "source_file", "", true},
		{SnippetContextFile, "var_declaration", "var value []int = call()", false},
	} {
		n := findMatchNode(t, doc.Root(), tc.kind, tc.text)
		if _, got := MatchNode(rootSlot(tc.context), n); got != tc.want {
			t.Errorf("%s slot against %s matched=%v want=%v", tc.context, tc.kind, got, tc.want)
		}
	}
}

func TestMatchTemplateEmptyListSlotsOwnAdjacentCommas(t *testing.T) {
	t.Parallel()
	cases := []struct {
		snippet string
		calls   []string
	}{
		{"f($before, pivot, $after)", []string{"f(pivot)", "f(a, pivot)", "f(pivot, b)", "f(a, pivot, b)"}},
		{"f(first, $middle, last)", []string{"f(first, last)", "f(first, x, last)", "f(first, x, y, last)"}},
	}
	for _, tc := range cases {
		tmpl := compileSnippetForTest(t, tc.snippet).Template()
		for _, call := range tc.calls {
			doc := parseGoForMatch(t, "package p\nvar _ = "+call+"\n")
			n := findMatchNode(t, doc.Root(), "call_expression", call)
			if _, ok := MatchNode(tmpl, n); !ok {
				t.Errorf("%q did not match %q", tc.snippet, call)
			}
		}
	}
}

func TestMatchTemplateRepeatedListIgnoresSeparatorsForEquality(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "func(){ $statements } == func(){ $statements }").Template()
	for _, tc := range []struct {
		expression string
		want       bool
	}{
		{"func(){ x(); y() } == func(){ x()\ny() }", true},
		{"func(){ x(); } == func(){ x() }", true},
		{"func(){ x()\ny() } == func(){ x()\ny() }", true},
		{"func(){ x(); y() } == func(){ x(); y() }", true},
		{"func(){ x(); } == func(){ x(); }", true},
	} {
		doc := parseGoForMatch(t, "package p\nvar _ = "+tc.expression+"\n")
		n := findMatchNode(t, doc.Root(), "binary_expression", tc.expression)
		if _, got := MatchNode(tmpl, n); got != tc.want {
			t.Errorf("%q matched=%v want=%v", tc.expression, got, tc.want)
		}
	}
}

func TestSequenceTargetValidatesParentAndSpan(t *testing.T) {
	t.Parallel()
	doc := parseGoForMatch(t, "package p\nfunc f(){ x(); y() }\n")
	list := findMatchNode(t, doc.Root(), "statement_list", "")
	tmpl := compileSnippetForTest(t, "x(); y()").Template()
	for name, target := range map[string]MatchTarget{
		"wrong parent category": SequenceTarget("declaration_sequence", list, 0, len(list.Children())),
		"negative start":        SequenceTarget("statement_sequence", list, -1, 1),
		"reversed":              SequenceTarget("statement_sequence", list, 2, 1),
		"past end":              SequenceTarget("statement_sequence", list, 0, len(list.Children())+1),
	} {
		if _, ok := MatchTemplate(tmpl, target); ok {
			t.Errorf("%s target matched", name)
		}
	}
}

func TestMatchTemplateConcurrentDocumentClose(t *testing.T) {
	tmpl := compileSnippetForTest(t, "f($args)").Template()
	for i := 0; i < 100; i++ {
		doc, err := parser.ParseDocument("go", "package p\nvar _ = f(a,b)\n")
		if err != nil {
			t.Fatal(err)
		}
		n := findMatchNode(t, doc.Root(), "call_expression", "f(a,b)")
		start := make(chan struct{})
		done := make(chan struct{})
		go func() {
			close(start)
			doc.Close()
			close(done)
		}()
		<-start
		result, ok := MatchNode(tmpl, n)
		<-done
		if ok && (result.Range().StartByte >= result.Range().EndByte || result.Bindings().Len() != 1) {
			t.Fatalf("close-interleaved match returned partial result: %+v", result)
		}
	}
}

func FuzzMatchTemplateNoPanicDeterministic(f *testing.F) {
	for _, seed := range []string{"a", "f(a,b)", "f(/*c*/a)", "x + y", "T{K: 1}"} {
		f.Add(seed)
	}
	tmpl, err := Compile([]byte("language go\n`f($args)`"), CompileOptions{})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, expression string) {
		doc, err := parser.ParseDocument("go", "package p\nvar _ = "+expression+"\n")
		if err != nil {
			return
		}
		defer doc.Close()
		root := doc.Root()
		a, oka := MatchNode(tmpl.Root().Template(), root)
		b, okb := MatchNode(tmpl.Root().Template(), root)
		if oka != okb || !reflect.DeepEqual(a.Range(), b.Range()) || a.Bindings().Len() != b.Bindings().Len() {
			t.Fatal("non-deterministic result")
		}
	})
}
