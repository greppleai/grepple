package gritql

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func compileSnippetForTest(t *testing.T, snippet string) Expression {
	t.Helper()
	p, err := Compile([]byte("language go\n`"+snippet+"`"), CompileOptions{})
	if err != nil {
		t.Fatalf("Compile(%q): %v", snippet, err)
	}
	return p.Root()
}

func TestGoTemplateContextsAndPriority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		snippet string
		context SnippetContext
		kind    string
	}{
		{"x", SnippetContextExpression, "identifier"},
		{"map[string]int", SnippetContextType, "map_type"},
		{"return x", SnippetContextStatement, "return_statement"},
		{"x++; y++", SnippetContextStatementList, "statement_sequence"},
		{"var x int", SnippetContextDeclaration, "var_declaration"},
		{"var x int; var y int", SnippetContextDeclarationList, "declaration_sequence"},
		{"package p\nvar x int", SnippetContextFile, "source_file"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.context.String(), func(t *testing.T) {
			e := compileSnippetForTest(t, tc.snippet)
			got := e.Template()
			if got.Context() != tc.context || got.Root().Kind() != tc.kind {
				t.Fatalf("template = %s %q", got.Context(), got.Root().Kind())
			}
		})
	}
}

func TestGoTemplateConcreteTokensFieldsAndTrivia(t *testing.T) {
	t.Parallel()
	e := compileSnippetForTest(t, "f(obj.Field, T{K: \\`raw\\`}) /* omitted */")
	tokens, fields := templateTokensAndFields(e.Template().Root())
	for _, want := range []string{"f", "(", "obj", ".", "Field", ",", "T", "{", "K", ":", "`", "raw", "`", "}", ")"} {
		if !containsString(tokens, want) {
			t.Errorf("missing token %q in %q", want, tokens)
		}
	}
	if !containsString(fields, "function") || !containsString(fields, "arguments") || !containsString(fields, "field") {
		t.Fatalf("fields = %q", fields)
	}
	for _, token := range tokens {
		if token == "/* omitted */" {
			t.Fatal("comment leaked into template")
		}
	}
}

func templateTokensAndFields(root TemplateNode) ([]string, []string) {
	var tokens []string
	var fields []string
	stack := []TemplateNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.Token() != "" {
			tokens = append(tokens, node.Token())
		}
		children := node.Children()
		for i := len(children) - 1; i >= 0; i-- {
			fields = append(fields, children[i].Field())
			if !children[i].IsSlot() {
				stack = append(stack, children[i].Node())
			}
		}
	}
	return tokens, fields
}

func TestGoTemplateSlotsIdentityCardinalityAndCollision(t *testing.T) {
	t.Parallel()
	const source = "language go\n`many($left, __grit_metavariable_0__, $right)`"
	p, err := Compile([]byte(source), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tmpl := p.Root().Template()
	slots, literalCollision := templateChildSlotsAndToken(tmpl.Root(), "__grit_metavariable_0__")
	if len(slots) != 2 || !literalCollision {
		t.Fatalf("slots=%d collision literal=%v", len(slots), literalCollision)
	}
	if slots[0].Variable().ID == slots[1].Variable().ID || slots[0].Cardinality() != SlotMany || slots[1].Cardinality() != SlotMany {
		t.Fatalf("slots = %+v %+v", slots[0].Variable(), slots[1].Variable())
	}

	anon := compileSnippetForTest(t, "f($_, $_)").Template()
	anonSlots, _ := templateChildSlotsAndToken(anon.Root(), "")
	if len(anonSlots) != 2 || !anonSlots[0].Variable().Anonymous || !anonSlots[1].Variable().Anonymous {
		t.Fatal("anonymous slots not independent")
	}

	whole := compileSnippetForTest(t, "$x").Template()
	if !whole.RootSlot().Valid() || whole.RootSlot().Variable().Name != "$x" {
		t.Fatalf("root slot = %+v", whole.RootSlot().Variable())
	}
	repeated, marker := templateSlotsAndMarker(compileSnippetForTest(t, "f($same, $same)").Template())
	if len(repeated) != 2 || repeated[0].Variable().ID != repeated[1].Variable().ID || repeated[0].Variable().Range == repeated[1].Variable().Range || marker {
		t.Fatalf("repeated occurrence restoration = %+v marker=%v", repeated, marker)
	}
}

func templateChildSlotsAndToken(root TemplateNode, token string) ([]TemplateSlot, bool) {
	var slots []TemplateSlot
	foundToken := false
	stack := []TemplateNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.Token() == token {
			foundToken = true
		}
		for _, child := range node.Children() {
			if child.IsSlot() {
				slots = append(slots, child.Slot())
			} else {
				stack = append(stack, child.Node())
			}
		}
	}
	return slots, foundToken
}

func TestGoTemplateDeclarationsImportsMethodsAndSemicolons(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{"import (\"a\"; alias \"b\")", "func (r T) M(x int) {}", "type T struct { X int }", "var _ = T{X: 1}"} {
		if !compileSnippetForTest(t, snippet).Template().Valid() {
			t.Fatalf("no template for %q", snippet)
		}
	}
	imports := compileSnippetForTest(t, "import ($path; alias $other)").Template()
	importSlots, _ := templateSlotsAndMarker(imports)
	if len(importSlots) != 2 || importSlots[0].Variable().Name != "$path" || importSlots[1].Variable().Name != "$other" {
		t.Fatalf("import slots = %+v", importSlots)
	}
	list := compileSnippetForTest(t, "x++; y++").Template().Root()
	if !templateHasToken(list, ";") {
		t.Fatal("explicit semicolon omitted")
	}
	auto := compileSnippetForTest(t, "x++\ny++").Template().Root()
	if templateHasToken(auto, ";") {
		t.Fatal("automatic semicolon preserved")
	}
	for _, snippet := range []string{"x++;", "var x int;"} {
		root := compileSnippetForTest(t, snippet).Template().Root()
		if root.Kind() != map[bool]string{true: "statement_sequence", false: "declaration_sequence"}[snippet[0] == 'x'] || !templateHasToken(root, ";") {
			t.Fatalf("%q leaked wrapper or semicolon: %q", snippet, root.Kind())
		}
	}
}

func TestGoTemplateInvalidSnippetStableDiagnostic(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{"", "package", "f(", "return )", "$x +"} {
		source := "language go\n`" + snippet + "`"
		_, err := Compile([]byte(source), CompileOptions{})
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Code != "PATTERN_INVALID_SNIPPET" || ce.Class != "pattern" || ce.Range == nil || ce.Range.StartByte != 0 || ce.Range.EndByte != len(source) {
			t.Errorf("%q: %#v", snippet, err)
		}
	}
}

func TestGoTemplateImmutableConcurrentAfterParserClose(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "f($args)").Template()
	children := tmpl.Root().Children()
	children[0] = TemplateChild{}
	if !tmpl.Root().Children()[0].Node().Valid() {
		t.Fatal("children accessor mutated template")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if tmpl.Context() != SnippetContextExpression || tmpl.Root().Kind() != "call_expression" {
					t.Error("unstable concurrent template read")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestGoTemplateEscapedSnippetOffsetAndDepthLimit(t *testing.T) {
	t.Parallel()
	const source = "language go\n`f(\\`raw\\`, $x)`"
	p, err := Compile([]byte(source), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ref := p.Root().Template().Root().Children()[1].Node().Children()[3].Slot().Variable()
	if ref.Name != "$x" || ref.Range.StartByte != len(source)-4 || ref.Range.EndByte != len(source)-2 {
		t.Fatalf("escaped-source slot range = %+v", ref.Range)
	}
	deep := "language go\n`((((((((x))))))))`"
	_, err = Compile([]byte(deep), CompileOptions{MaxDepth: 4})
	var ce *CompileError
	if !errors.As(err, &ce) || ce.Code != "LIMIT_PARSE_DEPTH" || ce.Class != "resource" || ce.Range != nil {
		t.Fatalf("template depth diagnostic = %#v", err)
	}
}

type typedPlaceholderCase struct {
	snippet string
	context SnippetContext
	root    string
	card    SlotCardinality
}

func TestGoTemplateTypedPlaceholderPositions(t *testing.T) {
	t.Parallel()
	cases := []typedPlaceholderCase{
		{"if ok { $stmt }", SnippetContextStatement, "if_statement", SlotMany},
		{"func f($params) {}", SnippetContextDeclaration, "function_declaration", SlotMany},
		{"func ($recv) M() {}", SnippetContextDeclaration, "method_declaration", SlotMany},
		{"type T struct { $fields }", SnippetContextDeclaration, "type_declaration", SlotMany},
		{"var ($specs)", SnippetContextDeclaration, "var_declaration", SlotMany},
		{"import $path", SnippetContextDeclaration, "import_declaration", SlotOne},
		{"import ($imports)", SnippetContextDeclaration, "import_declaration", SlotMany},
		{"import $alias /*c*/ \"pkg\"", SnippetContextDeclaration, "import_declaration", SlotOne},
		{"T{$elements}", SnippetContextExpression, "composite_literal", SlotMany},
		{"f($args)", SnippetContextExpression, "call_expression", SlotMany},
		{"package p\n$decl", SnippetContextFile, "source_file", SlotMany},
	}
	for _, tc := range cases {
		t.Run(tc.snippet, func(t *testing.T) {
			assertTypedPlaceholderCase(t, tc)
		})
	}
}

func assertTypedPlaceholderCase(t *testing.T, tc typedPlaceholderCase) {
	t.Helper()
	tmpl := compileSnippetForTest(t, tc.snippet).Template()
	if tmpl.Context() != tc.context || tmpl.Root().Kind() != tc.root {
		t.Fatalf("got context=%s root=%q", tmpl.Context(), tmpl.Root().Kind())
	}
	slots, marker := templateSlotsAndMarker(tmpl)
	if len(slots) != 1 || firstSlotCardinality(slots) != tc.card || marker {
		t.Fatalf("slots=%d card=%v generated-marker=%v", len(slots), firstSlotCardinality(slots), marker)
	}
}

func firstSlotCardinality(slots []TemplateSlot) SlotCardinality {
	if len(slots) > 0 {
		return slots[0].Cardinality()
	}
	return 0
}

func TestGoTemplateMixedOccurrenceRolesAndCardinality(t *testing.T) {
	t.Parallel()
	cases := []struct {
		snippet string
		want    map[string]SlotCardinality
	}{
		{"import $alias $path", map[string]SlotCardinality{"$alias": SlotOne, "$path": SlotOne}},
		{"package p\nvar x=$value\n$decl", map[string]SlotCardinality{"$value": SlotMany, "$decl": SlotMany}},
		{"import ($alias $path; $imports)", map[string]SlotCardinality{"$alias": SlotOne, "$path": SlotOne, "$imports": SlotMany}},
		{"package p\nvar ($specs)\nvar x=f($args)", map[string]SlotCardinality{"$specs": SlotMany, "$args": SlotMany}},
		{"func f($params) ($results) { return f($args) }", map[string]SlotCardinality{"$params": SlotMany, "$results": SlotMany, "$args": SlotMany}},
	}
	for _, tc := range cases {
		t.Run(tc.snippet, func(t *testing.T) {
			tmpl := compileSnippetForTest(t, tc.snippet).Template()
			slots, marker := templateSlotsAndMarker(tmpl)
			if marker || len(slots) != len(tc.want) {
				t.Fatalf("slots=%d want=%d generated-marker=%v", len(slots), len(tc.want), marker)
			}
			for _, slot := range slots {
				want, ok := tc.want[slot.Variable().Name]
				if !ok || slot.Cardinality() != want {
					t.Errorf("slot %q cardinality=%v want=%v", slot.Variable().Name, slot.Cardinality(), want)
				}
			}
		})
	}
}

func TestGoTemplateDirectSpecCardinality(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{"var $spec", "type $spec = int"} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if len(slots) != 1 || slots[0].Cardinality() != SlotOne || marker {
			t.Errorf("%q: slots=%d marker=%v", snippet, len(slots), marker)
		}
	}
	for _, snippet := range []string{"var ($spec)", "const ($spec)", "type ($spec)"} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if len(slots) != 1 || slots[0].Cardinality() != SlotMany || marker {
			t.Errorf("%q: slots=%d marker=%v", snippet, len(slots), marker)
		}
	}
}

func TestGoTemplateDirectRepeatedFieldCardinality(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{
		"var $names = 1",
		"var $names int",
		"const $names = 1",
		"func f($names int){}",
		"type T struct {$names int}",
		"switch x.(type) { case $types: return }",
		"F[$types, int]",
		"func F[$names any]() {}",
		"interface { $elements }",
		"interface { $types | int }",
	} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if len(slots) != 1 || slots[0].Cardinality() != SlotMany || marker {
			t.Errorf("%q: slots=%d card=%v marker=%v", snippet, len(slots), func() SlotCardinality {
				if len(slots) == 1 {
					return slots[0].Cardinality()
				}
				return 0
			}(), marker)
		}
	}
}

func TestGoTemplateInterfaceElementPromotesWholeTypeElem(t *testing.T) {
	t.Parallel()
	tmpl := compileSnippetForTest(t, "interface { $element }").Template()
	if tmpl.Root().Kind() != "interface_type" {
		t.Fatalf("root=%q", tmpl.Root().Kind())
	}
	var direct []TemplateSlot
	for _, child := range tmpl.Root().Children() {
		if child.IsSlot() {
			direct = append(direct, child.Slot())
		}
	}
	if len(direct) != 1 || direct[0].Cardinality() != SlotMany {
		t.Fatalf("interface element was not promoted to direct SlotMany: %+v", direct)
	}
	// A direct interface element slot can be replaced by either a method element
	// or a type element; the union form remains a repeated type_elem position.
	union := compileSnippetForTest(t, "interface { $types | int }").Template()
	slots, marker := templateSlotsAndMarker(union)
	if marker || len(slots) != 1 || slots[0].Cardinality() != SlotMany {
		t.Fatalf("interface union: slots=%d marker=%v", len(slots), marker)
	}
}
func TestGoTemplateComponentScanningComments(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{
		"var ($spec /* ; */)",
		"type ($spec /* ) */)",
		"import ($alias /* ; */ $path)",
		"var ($spec // ; )\n)",
	} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if marker || len(slots) == 0 {
			t.Errorf("%q: slots=%d marker=%v", snippet, len(slots), marker)
		}
	}

	// A block-comment newline has the same component-separating effect as a Go
	// newline; punctuation in a same-line comment remains lexical trivia.
	for _, snippet := range []string{
		"var (\n$first /* split\n*/\n$second /* ; ) */\n)",
		"var ($first /* split\n*/ $second)",
	} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if marker || len(slots) != 2 || slots[0].Cardinality() != SlotMany || slots[1].Cardinality() != SlotMany {
			t.Fatalf("%q: multiline block comment slots=%d marker=%v", snippet, len(slots), marker)
		}
	}
}

func TestStructuralSeparatorScanLexicalNewlines(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"($x `;\n)` tail)",
		"($x \"; )\" tail)",
		"($x ';' tail)",
		"($x '\\'' tail)",
		"($x /* ; ) */ tail)",
	} {
		at := strings.Index(source, "$x")
		r := splitComponentRange(scanStructuralSeparators(source), parser.Range{StartByte: 0, EndByte: len(source)}, at)
		if got := source[r.StartByte:r.EndByte]; got != source[1:len(source)-1] {
			t.Errorf("%q: component=%q", source, got)
		}
	}
	source := "($x /* first\nsecond\nthird */ tail)"
	at := strings.Index(source, "$x")
	r := splitComponentRange(scanStructuralSeparators(source), parser.Range{StartByte: 0, EndByte: len(source)}, at)
	if got := source[r.StartByte:r.EndByte]; got != "$x " {
		t.Fatalf("block-comment newline did not end the preceding component before the comment: %q", got)
	}
}

func TestGoTemplateManyIndependentMixedRoleComponents(t *testing.T) {
	t.Parallel()
	var snippet strings.Builder
	snippet.WriteString("package p\n")
	const groups = 12
	// Keep specialized and ordinary components interleaved so no successful
	// inference can depend on fixing a suffix or a favorable coordinate order.
	for i := range groups {
		fmt.Fprintf(&snippet, "import ($alias%d $path%d)\n", i, i)
		fmt.Fprintf(&snippet, "var ($spec%d)\n", i)
		fmt.Fprintf(&snippet, "var x%d = f($args%d)\n", i, i)
		fmt.Fprintf(&snippet, "$decl%d\n", i)
	}
	tmpl := compileSnippetForTest(t, snippet.String()).Template()
	slots, marker := templateSlotsAndMarker(tmpl)
	if marker || len(slots) != groups*5 {
		t.Fatalf("slots=%d want=%d marker=%v", len(slots), groups*5, marker)
	}
	counts := map[SlotCardinality]int{}
	for _, slot := range slots {
		counts[slot.Cardinality()]++
	}
	if counts[SlotOne] != groups*2 || counts[SlotMany] != groups*3 {
		t.Fatalf("cardinality counts=%v", counts)
	}
}

func TestGoTemplateManyOrdinaryPlaceholders(t *testing.T) {
	t.Parallel()
	var snippet strings.Builder
	snippet.WriteString("f(")
	const count = 200
	for i := range count {
		if i > 0 {
			snippet.WriteByte(',')
		}
		fmt.Fprintf(&snippet, "$arg%d", i)
	}
	snippet.WriteByte(')')
	tmpl := compileSnippetForTest(t, snippet.String()).Template()
	slots, marker := templateSlotsAndMarker(tmpl)
	if marker || len(slots) != count {
		t.Fatalf("slots=%d want=%d marker=%v", len(slots), count, marker)
	}
}
func TestGoTemplateLabelNames(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{"goto $label", "break $label", "$label: return"} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if len(slots) != 1 || slots[0].Cardinality() != SlotOne || marker {
			t.Errorf("%q: slots=%d marker=%v", snippet, len(slots), marker)
		}
	}
}

func TestGoTemplateIdentifierLikeNodeKinds(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{
		"package $pkg\nvar x int",
		"x.$field",
		"var x $type",
		"type $name int",
	} {
		tmpl := compileSnippetForTest(t, snippet).Template()
		slots, marker := templateSlotsAndMarker(tmpl)
		if len(slots) != 1 || slots[0].Cardinality() != SlotOne || marker {
			t.Errorf("%q: slots=%d marker=%v", snippet, len(slots), marker)
		}
	}
}

func TestGoTemplateRejectsUnrestoredOccurrences(t *testing.T) {
	t.Parallel()
	for _, snippet := range []string{"\"$x\"", "/* $x */ f()", "$x$y", "f(\"$x\")", "// $x\nf()"} {
		source := "language go\n`" + snippet + "`"
		_, err := Compile([]byte(source), CompileOptions{})
		var ce *CompileError
		if !errors.As(err, &ce) || ce.Code != "PATTERN_INVALID_SNIPPET" {
			t.Errorf("%q: %#v", snippet, err)
		}
	}
}
func TestProductionInferenceUsesOnlyGrammarDerivedAssignment(t *testing.T) {
	t.Parallel()
	d := decodedSnippet{text: "$decl", placeholders: []snippetPlaceholder{{start: 0, end: 5, ref: VariableRef{ID: 1}}}}
	source, generated := replaceSnippetPlaceholders(d, "", nil)
	doc, err := parser.ParseDocument("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	inference := inferPlaceholderRoles(doc.Root(), generated, source, 64)
	if !inference.ok || len(inference.assignments) != 1 {
		t.Fatalf("inference ok=%v assignments=%v", inference.ok, inference.assignments)
	}
	if len(inference.assignments[0]) != len(d.placeholders) {
		t.Fatalf("incomplete assignment: %v", inference.assignments[0])
	}
}

func TestTemplateCandidateDedupeAndAmbiguity(t *testing.T) {
	ref := VariableRef{ID: 7}
	one := Template{context: SnippetContextExpression, rootSlot: &templateSlot{ref: ref, cardinality: SlotOne}}
	same := Template{context: SnippetContextExpression, rootSlot: &templateSlot{ref: ref, cardinality: SlotOne}}
	different := Template{context: SnippetContextExpression, root: &templateNode{kind: "identifier", named: true, token: "x"}}
	if got := dedupeTemplates([]Template{one, same}); len(got) != 1 {
		t.Fatalf("dedupe=%d", len(got))
	}
	if got := dedupeTemplates([]Template{one, different}); len(got) != 2 {
		t.Fatalf("distinct=%d", len(got))
	}
	// The assignment constructor used by production groups correlated changes by
	// grammar-local component. The first component models an import alias/path pair:
	// its two occurrences must change together, never as Hamming-1 candidates.
	assignments := completeInferenceAssignments(
		[]placeholderRole{roleNode, roleImportPath, roleDeclaration},
		[]roleAlternative{
			{component: spanKey{10, 20}, index: 0, role: roleImportPath},
			{component: spanKey{10, 20}, index: 1, role: roleNode},
			{component: spanKey{30, 40}, index: 2, role: roleNode},
		},
	)
	if len(assignments) != 3 || assignments[1][0] != roleImportPath || assignments[1][1] != roleNode || assignments[1][2] != roleDeclaration {
		t.Fatalf("component assignments=%v", assignments)
	}
	candidates, tooDeep := collectInferredTemplates(3, assignments, func(roles []placeholderRole) inferredEvaluation {
		if roles[0] == roleImportPath || roles[2] == roleNode {
			return inferredEvaluation{tmpl: different, valid: true}
		}
		return inferredEvaluation{tmpl: one, valid: true}
	})
	if tooDeep || len(candidates) != 2 {
		t.Fatalf("inference candidates=%d tooDeep=%v", len(candidates), tooDeep)
	}
}

func templateSlotsAndMarker(t Template) ([]TemplateSlot, bool) {
	var slots []TemplateSlot
	marker := false
	if t.RootSlot().Valid() {
		slots = append(slots, t.RootSlot())
	}
	stack := []TemplateNode{t.Root()}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if strings.Contains(n.Token(), "__grit_slot_") {
			marker = true
		}
		for _, c := range n.Children() {
			if c.IsSlot() {
				slots = append(slots, c.Slot())
			} else {
				stack = append(stack, c.Node())
			}
		}
	}
	return slots, marker
}
func templateHasToken(root TemplateNode, want string) bool {
	stack := []TemplateNode{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Token() == want {
			return true
		}
		for _, c := range n.Children() {
			if !c.IsSlot() {
				stack = append(stack, c.Node())
			}
		}
	}
	return false
}
func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
