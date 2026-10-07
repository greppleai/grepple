package parser

import (
	"os"
	"testing"
)

func svelteFixture(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("../../testdata/svelte/Counter.svelte")
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestSvelteSyntaxOutlineAndCapabilities(t *testing.T) {
	service := NewParser()
	if service.LanguageFor("Counter.svelte") != "svelte" || service.LanguageFor("Counter.svelte.js") != "javascript" {
		t.Fatal("incorrect Svelte extension classification")
	}
	document, err := service.Parse("svelte", svelteFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if document.tree.RootNode().HasError() {
		t.Fatal("valid Svelte component parsed with errors")
	}
	outline := service.Outline(document, "Counter.svelte")
	mustFind(t, outline.Symbols, "script", "script")
	mustFind(t, outline.Symbols, "style", "style")
	snippet := mustFind(t, outline.Symbols, "snippet", "label")
	mustFind(t, snippet.Children, "element", "span")
	section := mustFind(t, outline.Symbols, "element", "section")
	mustFind(t, section.Children, "block", "if")
	mustFind(t, section.Children, "block", "each")
	button := mustFind(t, section.Children, "element", "Button")
	if len(button.Children) != 0 {
		t.Fatalf("self-closing component duplicated in outline: %+v", button)
	}
	capability, ok := service.CapabilitiesForLanguage("svelte")
	if !ok || !capability.NavigationFacts.Declarations || !capability.NavigationFacts.Calls || !capability.ImportNavigation || capability.EntrypointNavigation {
		t.Fatalf("unexpected Svelte navigation capabilities: %+v", capability)
	}
}

func TestSvelteNavigationIncludesNativeScriptAndStyleFacts(t *testing.T) {
	graph := BuildNavigationGraph(svelteFixture(t), "svelte", "Counter.svelte")
	if !svelteHasSnippet(graph.Declarations, "label") {
		t.Fatalf("unexpected snippet declarations: %+v", graph.Declarations)
	}
	if len(graph.Imports) != 1 || graph.Imports[0].ImportPath != "./Button.svelte" {
		t.Fatalf("embedded script imports missing: %+v", graph)
	}
}

func TestSvelteStructuralSegmentsPreserveScriptAndTemplateContext(t *testing.T) {
	content := svelteFixture(t)
	for _, line := range []int{4, 15, 27} {
		segments, status := BuildSegmentsWithStatus(content, "svelte", map[int]bool{line: true})
		if status != SegmentBuildStructured || len(segments) == 0 {
			t.Fatalf("line %d: status=%s segments=%+v", line, status, segments)
		}
		covered := false
		for _, segment := range segments {
			covered = covered || segment.Start <= line && segment.End >= line
		}
		if !covered {
			t.Fatalf("line %d omitted from structural context: %+v", line, segments)
		}
	}
}

func TestSvelteMalformedTemplateReportsRecovery(t *testing.T) {
	segments, status := BuildSegmentsWithStatus("{#if ready}\n<button>Ready</button>\n", "svelte", map[int]bool{2: true})
	if status != SegmentBuildRecovered || len(segments) == 0 {
		t.Fatalf("malformed template status=%s segments=%+v", status, segments)
	}
}

func TestSvelteAwaitKeyAndSpecialElementOutlines(t *testing.T) {
	content := `<svelte:head><title>Example</title></svelte:head>
{#key identity}
  {#await promise}
    <p>Loading</p>
  {:then value}
    <p>{value}</p>
  {:catch error}
    <p>{error.message}</p>
  {/await}
{/key}
`
	document, err := NewParser().Parse("svelte", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if document.tree.RootNode().HasError() {
		t.Fatal("valid await/key blocks parsed with errors")
	}
	outline := NewParser().Outline(document, "App.svelte")
	mustFind(t, outline.Symbols, "element", "svelte:head")
	key := mustFind(t, outline.Symbols, "block", "key")
	await := mustFind(t, key.Children, "block", "await")
	mustFind(t, await.Children, "block", "then")
	mustFind(t, await.Children, "block", "catch")
}

func svelteHasSnippet(declarations []NavigationDeclaration, name string) bool {
	for _, declaration := range declarations {
		if declaration.Name == name && declaration.Kind == "snippet" {
			return true
		}
	}
	return false
}
