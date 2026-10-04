package parser

import (
	"os"
	"testing"
)

func cssFixture(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../testdata/css/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestCSSSyntaxOutlineAndCapabilities(t *testing.T) {
	service := NewParser()
	for path, want := range map[string]string{"styles.css": "css", "styles.CSS": "css", "styles.scss": "text", "styles.less": "text", "styles.css.js": "javascript"} {
		if got := service.LanguageFor(path); got != want {
			t.Fatalf("%s: %s, want %s", path, got, want)
		}
	}
	document, err := service.Parse("css", cssFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if document.tree.RootNode().HasError() {
		t.Fatal("valid CSS parsed with errors")
	}
	outline := service.Outline(document, "styles.css")
	root := mustFind(t, outline.Symbols, "rule", ":root")
	mustFind(t, root.Children, "property", "--accent")
	button := mustFind(t, outline.Symbols, "rule", ".button, .card:hover")
	mustFind(t, button.Children, "property", "color")
	mustFind(t, button.Children, "rule", "& > span")
	media := mustFind(t, outline.Symbols, "block", "@media (min-width: 40rem)")
	mustFind(t, media.Children, "rule", ".button")
	frames := mustFind(t, outline.Symbols, "keyframes", "fade")
	mustFind(t, frames.Children, "block", "from")
	mustFind(t, frames.Children, "block", "to")
	capability, ok := service.CapabilitiesForLanguage("css")
	if !ok || !capability.NavigationFacts.Declarations || capability.NavigationFacts.Calls || capability.ImportNavigation || capability.EntrypointNavigation {
		t.Fatalf("unexpected CSS capabilities: %+v", capability)
	}
}

func TestCSSNavigationKeepsConfigurationFactsConservative(t *testing.T) {
	graph := BuildNavigationGraph(cssFixture(t), "css", "styles.css")
	seen := map[string]bool{}
	for _, declaration := range graph.Declarations {
		seen[declaration.Name] = true
	}
	for _, name := range []string{":root", "--accent", ".button, .card:hover", "color", "fade"} {
		if !seen[name] {
			t.Fatalf("missing declaration %q: %+v", name, graph.Declarations)
		}
	}
	if len(graph.Calls) != 0 || len(graph.Imports) != 0 || len(graph.TypeDeclarations) != 0 {
		t.Fatalf("CSS must not invent call/import/type resolution: %+v", graph)
	}
}

func TestCSSStructuralContextAndMalformedRecovery(t *testing.T) {
	for _, line := range []int{4, 11, 16, 29} {
		segments, status := BuildSegmentsWithStatus(cssFixture(t), "css", map[int]bool{line: true})
		if status != SegmentBuildStructured || len(segments) == 0 {
			t.Fatalf("line %d: %s %+v", line, status, segments)
		}
		covered := false
		for _, segment := range segments {
			covered = covered || segment.Start <= line && segment.End >= line
		}
		if !covered {
			t.Fatalf("line %d omitted: %+v", line, segments)
		}
	}
	segments, status := BuildSegmentsWithStatus(".button {\n color: red;\n", "css", map[int]bool{2: true})
	if status != SegmentBuildRecovered || len(segments) == 0 {
		t.Fatalf("malformed CSS: %s %+v", status, segments)
	}
}
