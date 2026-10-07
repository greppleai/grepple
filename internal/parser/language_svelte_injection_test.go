package parser

import (
	"strings"
	"sync"
	"testing"
)

func TestSvelteEmbeddedTreesKeepOriginalCoordinatesAndOutlines(t *testing.T) {
	source := "<!-- café -->\r\n<script lang='ts'>export function greet(name: string) { return name; }</script>\r\n<style>.café { color: red; }</style>\r\n<p>Hello</p>"
	document, err := NewParser().Parse("svelte", source)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if diagnostics := document.ParseDiagnostics(); len(diagnostics) != 0 {
		t.Fatalf("valid component: %+v", diagnostics)
	}
	var function, property Node
	document.WalkNamed(func(node Node) {
		switch node.Kind() {
		case "function_declaration":
			function = node
		case "declaration":
			property = node
		}
	})
	assertSvelteEmbeddedNodeCoordinates(t, source, function, property)
	if function.Range().Start.Line != 2 || property.Range().Start.Line != 3 {
		t.Fatal("incorrect embedded line mapping")
	}
	outline := NewParser().Outline(document, "App.svelte")
	script := mustFind(t, outline.Symbols, "script", "script")
	mustFind(t, script.Children, "function", "greet")
	style := mustFind(t, outline.Symbols, "style", "style")
	rule := mustFind(t, style.Children, "rule", ".café")
	mustFind(t, rule.Children, "property", "color")
	if !function.Parent().Parent().Valid() {
		t.Fatal("embedded root not attached to markup")
	}
	document.Close()
	if function.Valid() || property.Valid() || function.Text() != "" {
		t.Fatal("embedded handles survive owner close")
	}
}

func TestSvelteEmbeddedDiagnosticsAndUnsupportedLanguages(t *testing.T) {
	for _, source := range []string{
		"<script>function broken( {</script>",
		"<script>let value: string = 'x';</script>",
		"<style>.button { color: ; </style>",
		"<style lang='scss'>$value: red;</style>",
		"<script lang={unknown}>const value = 1;</script>",
	} {
		document, err := NewParser().Parse("svelte", source)
		if err != nil {
			t.Fatal(err)
		}
		assertSvelteEmbeddedDiagnostics(t, document, source)
		document.Close()
	}
	for _, source := range []string{"<script lang='typescript'>let value: string = 'x';</script>", "<script src='./external.js'></script>", "<script>const text = '<style>.fake { color: red; }</style>';</script>"} {
		document, err := NewParser().Parse("svelte", source)
		if err != nil {
			t.Fatal(err)
		}
		if diagnostics := document.ParseDiagnostics(); len(diagnostics) != 0 {
			t.Fatalf("valid source rejected: %+v", diagnostics)
		}
		document.Close()
	}
}

func TestSvelteEmbeddedViewConcurrentReadAndClose(t *testing.T) {
	source := "<script>function greet() { return 1; }</script><style>.x { color: red; }</style>"
	document, err := NewParser().Parse("svelte", source)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			for range 20 {
				readSvelteEmbeddedView(t, document, source)
			}
		})
	}
	document.Close()
	wait.Wait()
}

func assertSvelteEmbeddedNodeCoordinates(t *testing.T, source string, nodes ...Node) {
	t.Helper()
	for _, node := range nodes {
		if !node.Valid() {
			t.Fatal("embedded node missing")
		}
		rangeValue := node.Range()
		if source[rangeValue.StartByte:rangeValue.EndByte] != node.Text() {
			t.Fatalf("incorrect byte mapping: %+v %q", rangeValue, node.Text())
		}
		if node.Parent().Kind() == "" {
			t.Fatal("embedded node lost parent")
		}
	}
}
func assertSvelteEmbeddedDiagnostics(t *testing.T, document *Document, source string) {
	t.Helper()
	diagnostics := document.ParseDiagnostics()
	if len(diagnostics) == 0 {
		t.Fatalf("bad embedded source accepted: %s", source)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Range.StartByte < 0 || diagnostic.Range.EndByte > len(source) {
			t.Fatalf("diagnostic escaped component: %+v", diagnostic)
		}
	}
}
func readSvelteEmbeddedView(t *testing.T, document *Document, source string) {
	t.Helper()
	_ = document.Read(func(view DocumentView) error {
		walkSvelteEmbeddedView(t, view.Root(), source)
		return nil
	})
}
func walkSvelteEmbeddedView(t *testing.T, node ViewNode, source string) {
	t.Helper()
	for _, child := range node.Children() {
		if !strings.Contains(source, child.Text()) {
			t.Error("foreign text in view")
		}
		walkSvelteEmbeddedView(t, child, source)
	}
}

func TestSvelteEmbeddedCacheIdentityAndPreciseContext(t *testing.T) {
	capability, ok := NewParser().CapabilitiesForLanguage("svelte")
	if !ok || capability.GrammarFingerprint == generatedLanguageMetadata["svelte"].fingerprint {
		t.Fatal("embedded grammars omitted from cache identity")
	}
	source := "<script>\nfunction ignored() { return 0; }\nfunction selected() {\n  return 1;\n}\n</script>\n<style>\n.x { color: red; }\n</style>"
	segments, status := BuildSegmentsWithStatus(source, "svelte", map[int]bool{4: true})
	if status != SegmentBuildStructured {
		t.Fatalf("status: %s", status)
	}
	assertSveltePreciseContext(t, segments)
	document, err := NewParser().Parse("svelte", source)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	for range 2 {
		graph := cachedNavigationGraphFromDocument(document, "App.svelte")
		if len(graph.Declarations) < 4 {
			t.Fatalf("embedded facts missing from cache: %+v", graph)
		}
	}
}

func assertSveltePreciseContext(t *testing.T, segments []Segment) {
	t.Helper()
	for _, segment := range segments {
		if segment.Start <= 2 && segment.End >= 2 {
			t.Fatalf("unrelated function retained: %+v", segments)
		}
	}
	for _, line := range []int{1, 3, 4, 5, 6} {
		covered := false
		for _, segment := range segments {
			covered = covered || segment.Start <= line && segment.End >= line
		}
		if !covered {
			t.Fatalf("required context line %d missing: %+v", line, segments)
		}
	}
}
