package gritql

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestParseQueryClosedGrammar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		kind string
	}{
		{"snippet", "language go\n`exec.Command($args)`", "codeSnippet"},
		{"regex constraint", "language go\n`f($x)` where { $x <: r\"^x$\", }", "patternWhere"},
		{"empty constraint", "language go\n`f($args)` where { $args <: empty }", "patternWhere"},
		{"boolean", "language go\nand { `x`, not `y`, }", "patternAnd"},
		{"direct function parent", "language go\nand { `{}`, not parent kind(\"function\"), }", "patternAnd"},
		{"right associative", "language go\nnot maybe `x`", "patternNot"},
		{"comments CRLF", "language go // header\r\n// lead\r\ncontains // separator\r\n`茶` // eof", "patternContains"},
		{"snippet escapes", "language go\n`var s = \\`raw\\``", "codeSnippet"},
		{"regex escapes", "language go\n`f($x)` where { $x <: r\"\\n\\r\\t\\x41\\u0042\\\\\\\"\" }", "patternWhere"},
		{"NUL and BOM snippet scalars", "language go\n`a\x00\ufeffb`", "codeSnippet"},
		{"NUL comment scalar", "language go // a\x00b\n`x`", "codeSnippet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseQuery([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			defer doc.close()
			if !bytes.Equal(doc.bytes(), []byte(tt.src)) {
				t.Fatal("document did not retain exact bytes")
			}
			pattern := doc.root().childByFieldName("pattern")
			if got := pattern.kind(); got != tt.kind {
				t.Fatalf("pattern kind = %q, want %q; source: %q", got, tt.kind, tt.src)
			}
			if got := doc.rangeForBytes(0, len(tt.src)); got.Start.Line != 1 || got.Start.Column != 1 {
				t.Fatalf("bad whole range: %+v", got)
			}
		})
	}
}

func TestParseQueryRejectsMalformedWholeRange(t *testing.T) {
	t.Parallel()
	tests := []string{
		"`x`",
		"language go",
		"language go\r`x`",
		"language go\n",
		"language go\nand { `x` }",
		"language go\nor { `x` `y` }",
		"language go\n`\\n`",
		"language go\nr\"\\q\"",
		"language go\n`x` where {$x <: `x`} where {$x <: `x`}",
		"language go\n`f($1x)`",
		"language go\nand { `x`, }",
		"language go\nand { `x`, `y` `z` }",
		"language go\n\ufeff`x`",
		"language go\n`x\r`",
		"language go\n`x`\x00",
		"language go\nfrobnicate",
	}
	for _, src := range tests {
		doc, err := parseQuery([]byte(src))
		if doc != nil {
			defer doc.close()
		}
		var syntaxErr *querySyntaxError
		if !errors.As(err, &syntaxErr) || syntaxErr.Kind != queryMalformed {
			t.Errorf("parseQuery(%q) error = %#v, want malformed", src, err)
			continue
		}
		if syntaxErr.Range.StartByte != 0 || syntaxErr.Range.EndByte != len(src) {
			t.Errorf("parseQuery(%q) range = %+v", src, syntaxErr.Range)
		}
	}
	invalid := []byte("language go\n`x`\xff")
	_, err := parseQuery(invalid)
	var syntaxErr *querySyntaxError
	if !errors.As(err, &syntaxErr) || syntaxErr.Kind != queryMalformed || syntaxErr.Range.EndByte != len(invalid) {
		t.Fatalf("invalid UTF-8 error = %#v", err)
	}
}

func TestParseQueryRecognizesUnsupported(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"language go\n`x` => `y`",
		"language go\npattern foo() { `x` }",
		"language go\nimport \"lib\"",
		"language go /* unsupported comment */\n`x`",
		"language go;\n`x`",
		"language go\nmultifile { `x`, `y` }",
		"language go\nCall(name=`x`)",
		"language go\ngo`x`",
		"language go\n`x` as $x",
		"language go\n$x = foo()",
		"language go\n`x` where { $x == `x` }",
		"language go\n(`x`)",
	} {
		doc, err := parseQuery([]byte(src))
		if doc == nil {
			t.Errorf("parseQuery(%q) discarded recognized tree", src)
			continue
		}
		defer doc.close()
		var syntaxErr *querySyntaxError
		if !errors.As(err, &syntaxErr) || syntaxErr.Kind != queryUnsupported {
			t.Errorf("parseQuery(%q) error = %#v, want unsupported", src, err)
		}
	}
}

func TestQueryRangesConcreteChildrenAndClose(t *testing.T) {
	t.Parallel()
	src := []byte("language go\r\ncontains `茶`\r\n")
	doc, err := parseQuery(src)
	if err != nil {
		t.Fatal(err)
	}
	n := doc.root().childByFieldName("pattern")
	if n.kind() != "patternContains" || len(n.children()) == 0 || n.children()[0].text() != "contains" {
		t.Fatalf("concrete node facade lost order/text: kind=%q source=%q", n.kind(), src)
	}
	snippet := n.childByFieldName("contains")
	r := snippet.byteRange()
	if r.Start.Line != 2 || r.Start.Column != 10 || r.End.Column != 13 || snippet.text() != "`茶`" {
		t.Fatalf("snippet range/text = %+v %q", r, snippet.text())
	}
	doc.close()
	doc.close()
	if doc.root().valid() || n.valid() || doc.bytes() != nil || doc.rangeForBytes(0, len(src)) != (queryRange{}) {
		t.Fatal("closed document or retained node remains valid")
	}
}

func TestParseQueryContextAndEnvelope(t *testing.T) {
	t.Parallel()

	doc, err := parseQuery([]byte("language go\n`f($_)` where { $_ <: `x` }"))
	if doc == nil {
		t.Fatal("invalid-context query discarded parse tree")
	}
	defer doc.close()
	var syntaxErr *querySyntaxError
	if !errors.As(err, &syntaxErr) || syntaxErr.Kind != queryInvalidContext {
		t.Fatalf("anonymous constraint error = %#v, want invalid context", err)
	}

	valid := []string{
		" // before header\nlanguage\tgo // header\r\ncontains // required separator\r\n`x`",
		"language go\nand {`x`, `y`}",
		"language go\nor {`x`, `y`, `z`}",
	}
	for _, src := range valid {
		parsed, parseErr := parseQuery([]byte(src))
		if parseErr != nil {
			t.Errorf("parseQuery(%q): %v", src, parseErr)
		}
		if parsed != nil {
			parsed.close()
		}
	}
	malformed := []string{
		"language go\nnot`x`",
		"language go\nand{}",
		"language go\nor {`x`,}",
		"language go\nlanguage go\n`x`",
		"language go\n`f($é)`",
		"language go\n`f($_x)`",
	}
	for _, src := range malformed {
		parsed, parseErr := parseQuery([]byte(src))
		if parsed != nil {
			parsed.close()
		}
		if !errors.As(parseErr, &syntaxErr) || syntaxErr.Kind != queryMalformed {
			t.Errorf("parseQuery(%q) error = %#v, want malformed", src, parseErr)
		}
	}
}

func TestParseQueryConcurrentDeterministic(t *testing.T) {
	t.Parallel()
	const source = "language go\nand { contains `茶`, not `y`, }"
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go runDeterministicQueryWorker(&wg, errs, source, 20)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func runDeterministicQueryWorker(wg *sync.WaitGroup, errs chan<- error, source string, iterations int) {
	defer wg.Done()
	for range iterations {
		if err := parseQueryWithExpectedRoot(source, "patternAnd"); err != nil {
			errs <- err
			return
		}
	}
}

func parseQueryWithExpectedRoot(source, expectedKind string) error {
	doc, err := parseQuery([]byte(source))
	if err == nil && doc.root().childByFieldName("pattern").kind() != expectedKind {
		err = errors.New("non-deterministic root")
	}
	if doc != nil {
		doc.close()
	}
	return err
}

func TestQueryDocumentConcurrentReadAndClose(t *testing.T) {
	t.Parallel()
	doc, err := parseQuery([]byte("language go\ncontains `茶`"))
	if err != nil {
		t.Fatal(err)
	}
	node := doc.root().childByFieldName("pattern")
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = doc.bytes()
				_ = doc.rangeForBytes(0, 1)
				_ = node.kind()
				_ = node.text()
				_ = node.children()
			}
		}()
	}
	doc.close()
	wg.Wait()
	if node.valid() {
		t.Fatal("node remained valid after close")
	}
}

func TestParseQueryHardDepthCap(t *testing.T) {
	t.Parallel()
	source := "language go\n" + strings.Repeat("not ", hardMaxDepth+1) + "`x`"
	doc, err := parseQuery([]byte(source))
	if doc != nil {
		defer doc.close()
	}
	var syntaxErr *querySyntaxError
	if !errors.As(err, &syntaxErr) || syntaxErr.Kind != queryDepthLimit {
		t.Fatalf("parseQuery hard depth error = %#v", err)
	}
}
