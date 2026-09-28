package parser

import (
	"strings"
	"sync"
	"testing"
)

func TestParserTraversesGoWithoutExposingTreeSitter(t *testing.T) {
	const source = "package p\n\nfunc Add(a, b int) int { return a + b }\n"
	doc, err := NewParser().Parse("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	assertDocumentMetadata(t, doc, source)
	root := requireGoDocumentRoot(t, doc, source)
	function := requireFunctionDeclaration(t, root)
	assertFunctionChildren(t, function)
	assertFunctionFields(t, function)
}

func assertDocumentMetadata(t *testing.T, doc *Document, source string) {
	t.Helper()
	if got := doc.Language(); got != "go" {
		t.Fatalf("Language() = %q", got)
	}
	if got := doc.Source(); got != source {
		t.Fatalf("Source() = %q", got)
	}
}

func requireGoDocumentRoot(t *testing.T, doc *Document, source string) Node {
	t.Helper()
	root := doc.Root()
	if !root.Valid() || root.Kind() != "source_file" || root.Text() != source {
		t.Fatalf("bad root: valid=%v kind=%q text=%q", root.Valid(), root.Kind(), root.Text())
	}
	children := root.Children()
	if len(children) == 0 || children[0].Kind() != "package_clause" {
		t.Fatalf("unexpected children: %#v", nodeKinds(children))
	}
	return root
}

func requireFunctionDeclaration(t *testing.T, root Node) Node {
	t.Helper()
	function := findNode(root, "function_declaration")
	if !function.Valid() || function.Parent().Kind() != "source_file" {
		t.Fatalf("function or parent unavailable")
	}
	return function
}

func assertFunctionChildren(t *testing.T, function Node) {
	t.Helper()
	all, named := function.Children(), function.NamedChildren()
	if len(all) <= len(named) || !containsKind(all, "func") {
		t.Fatalf("all children should include unnamed tokens: all=%v named=%v", nodeKinds(all), nodeKinds(named))
	}
	parameterChildren := function.ChildByFieldName("parameters").Children()
	if !containsKind(parameterChildren, "(") || !containsKind(parameterChildren, ")") {
		t.Fatalf("all children omit punctuation or order: %v", nodeKinds(parameterChildren))
	}
}

func assertFunctionFields(t *testing.T, function Node) {
	t.Helper()
	if got := function.ChildByFieldName("name"); !got.Valid() || got.Text() != "Add" || got.FieldName() != "name" {
		t.Fatalf("name field = valid %v text %q field %q", got.Valid(), got.Text(), got.FieldName())
	}
	for i, child := range function.Children() {
		if child.FieldName() != function.FieldNameForChild(i) {
			t.Fatalf("child %d field mismatch: %q != %q", i, child.FieldName(), function.FieldNameForChild(i))
		}
	}
	if got := function.ChildByFieldName("does_not_exist"); got.Valid() {
		t.Fatal("unknown field returned a node")
	}
}

func TestDocumentRangesUseBytesAndOneBasedUnicodeColumns(t *testing.T) {
	const source = "package p\r\nvar café = \"λ\"\r\n"
	doc, err := NewParser().Parse("go", source)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	identifier := findNodeWithText(doc.Root(), "identifier", "café")
	if !identifier.Valid() {
		t.Fatal("missing café identifier")
	}
	r := identifier.Range()
	if r.StartByte != strings.Index(source, "café") || r.EndByte-r.StartByte != len("café") {
		t.Fatalf("byte range = %#v", r)
	}
	if r.Start.Line != 2 || r.Start.Column != 5 || r.End.Line != 2 || r.End.Column != 9 {
		t.Fatalf("unicode range = %#v", r)
	}
	rootRange := doc.Root().Range()
	if rootRange.Start.Line != 1 || rootRange.Start.Column != 1 || rootRange.End.Line != 3 || rootRange.End.Column != 1 {
		t.Fatalf("root CRLF range = %#v", rootRange)
	}
}

func TestParserDiagnosticsAreDeterministic(t *testing.T) {
	doc, err := NewParser().Parse("go", "package p\nfunc f( {\n")
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	if !doc.Root().HasError() {
		t.Fatal("malformed root does not report an error")
	}
	first, second := doc.ParseDiagnostics(), doc.ParseDiagnostics()
	if len(first) == 0 {
		t.Fatal("expected parse diagnostics")
	}
	if len(first) != len(second) {
		t.Fatalf("diagnostics changed: %#v %#v", first, second)
	}
	for i := range first {
		if first[i] != second[i] || first[i].Message == "" || first[i].Range.Start.Line < 1 {
			t.Fatalf("bad diagnostic %d: %#v / %#v", i, first[i], second[i])
		}
	}
}

func TestParserRejectsUnsupportedLanguage(t *testing.T) {
	parseInvocations.Store(0)
	doc, err := NewParser().Parse("not-a-language", "anything")
	if err == nil || doc != nil {
		t.Fatalf("Parser.Parse = (%v, %v), want nil error result", doc, err)
	}
	if got := parseInvocations.Load(); got != 0 {
		t.Fatalf("unsupported language invoked parser %d times", got)
	}
}

func TestParserRejectsInvalidUTF8BeforeParsing(t *testing.T) {
	parseInvocations.Store(0)
	doc, err := NewParser().Parse("go", string([]byte{'p', 0xff}))
	if err == nil || doc != nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Parser.Parse = (%v, %v), want invalid UTF-8 error", doc, err)
	}
	if got := parseInvocations.Load(); got != 0 {
		t.Fatalf("invalid UTF-8 invoked parser %d times", got)
	}
}

func TestDocumentCloseIsIdempotentAndInvalidatesNodes(t *testing.T) {
	doc, err := NewParser().Parse("go", "package p\n")
	if err != nil {
		t.Fatal(err)
	}
	root, child := doc.Root(), doc.Root().NamedChildren()[0]
	doc.Close()
	doc.Close()

	if doc.Root().Valid() || root.Valid() || child.Valid() {
		t.Fatal("nodes remain valid after close")
	}
	if root.Kind() != "" || root.Text() != "" || root.Parent().Valid() || len(root.Children()) != 0 || root.Range() != (Range{}) {
		t.Fatal("closed node methods did not return zero values")
	}
	if doc.Source() != "" || doc.Language() != "" || len(doc.ParseDiagnostics()) != 0 {
		t.Fatal("closed document methods did not return zero values")
	}
	var zero Node
	if zero.Valid() || zero.IsNamed() || zero.IsExtra() || zero.IsError() || zero.IsMissing() || zero.HasError() {
		t.Fatal("zero node is not safely invalid")
	}
}

func TestParserUsesOneParseAndSupportsConcurrentDocuments(t *testing.T) {
	parseInvocations.Store(0)
	const count = 16
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doc, err := NewParser().Parse("go", "package p\nfunc f() {}\n")
			if err != nil {
				errs <- err
				return
			}
			if doc.Root().Kind() != "source_file" {
				errs <- errUnexpectedRoot{}
			}
			doc.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if got := parseInvocations.Load(); got != count {
		t.Fatalf("parse invocations = %d, want %d", got, count)
	}
}

func TestDocumentConcurrentCloseAndRead(t *testing.T) {
	doc, err := NewParser().Parse("go", "package p\nfunc f() {}\n")
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Root()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = root.Kind()
				_ = root.Text()
				_ = root.Children()
			}
		}()
	}
	doc.Close()
	wg.Wait()
	if root.Valid() {
		t.Fatal("root valid after concurrent close")
	}
}

func TestNodeSnapshotSurvivesDocumentClose(t *testing.T) {
	doc, err := NewParser().Parse("go", "package p\nvar x = f(a, b)\n")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := doc.Root().Snapshot()
	if !ok {
		t.Fatal("snapshot failed")
	}
	doc.Close()
	if !snapshot.Valid() || snapshot.Kind() != "source_file" || snapshot.Text() == "" {
		t.Fatalf("snapshot invalid after close: kind=%q text=%q", snapshot.Kind(), snapshot.Text())
	}
	children := snapshot.Children()
	if len(children) == 0 {
		t.Fatal("snapshot descendants missing after close")
	}
	children[0] = SyntaxNode{}
	if !snapshot.Children()[0].Valid() {
		t.Fatal("Children exposed mutable snapshot storage")
	}
}

type errUnexpectedRoot struct{}

func (errUnexpectedRoot) Error() string { return "unexpected root" }

func nodeKinds(nodes []Node) []string {
	result := make([]string, len(nodes))
	for i := range nodes {
		result[i] = nodes[i].Kind()
	}
	return result
}
func containsKind(nodes []Node, kind string) bool {
	for _, node := range nodes {
		if node.Kind() == kind {
			return true
		}
	}
	return false
}
func TestWalkNamedUsesStablePreorder(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\nfunc run() { call() }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	var kinds []string
	document.WalkNamed(func(node Node) { kinds = append(kinds, node.Kind()) })
	if len(kinds) < 4 || kinds[0] != "source_file" || kinds[1] != "package_clause" || kinds[2] != "package_identifier" {
		t.Fatalf("walk order=%v", kinds)
	}
}

func findNode(root Node, kind string) Node {
	if root.Kind() == kind {
		return root
	}
	for _, child := range root.NamedChildren() {
		if found := findNode(child, kind); found.Valid() {
			return found
		}
	}
	return Node{}
}
func findNodeWithText(root Node, kind, text string) Node {
	if root.Kind() == kind && root.Text() == text {
		return root
	}
	for _, child := range root.NamedChildren() {
		if found := findNodeWithText(child, kind, text); found.Valid() {
			return found
		}
	}
	return Node{}
}
