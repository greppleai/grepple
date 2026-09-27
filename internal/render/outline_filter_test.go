package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestParseOutlineKinds(t *testing.T) {
	kinds, err := ParseOutlineKinds([]string{"types, functions", "variables"}, true)
	if err != nil || !reflect.DeepEqual(kinds, OutlineKinds{"types": true, "functions": true, "variables": true}) {
		t.Fatalf("kinds=%v err=%v", kinds, err)
	}
	for _, test := range []struct {
		values  []string
		outline bool
		message string
	}{
		{[]string{"types"}, false, "--kind requires --outline"},
		{[]string{"other"}, true, "invalid --kind"},
		{[]string{"types,"}, true, "invalid --kind"},
	} {
		if _, err := ParseOutlineKinds(test.values, test.outline); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("ParseOutlineKinds(%v, %t) = %v; want %q", test.values, test.outline, err, test.message)
		}
	}
}

func TestFilterOutlineLiftsNestedMatchesWithoutMutatingInput(t *testing.T) {
	original := parser.FileOutline{Path: "file.cs", Language: "csharp", Symbols: []parser.Symbol{
		{Kind: "namespace", Name: "App", Children: []parser.Symbol{
			{Kind: "class", Name: "Widget", Children: []parser.Symbol{
				{Kind: "method", Name: "Run"},
				{Kind: "field", Name: "ID"},
			}},
		}},
		{Kind: "function", Name: "Other"},
	}}
	functions := FilterOutline(original, OutlineKinds{"functions": true})
	if len(functions.Symbols) != 2 || functions.Symbols[0].Name != "Run" || functions.Symbols[1].Name != "Other" || len(functions.Symbols[0].Children) != 0 {
		t.Fatalf("functions=%#v", functions.Symbols)
	}
	types := FilterOutline(original, OutlineKinds{"types": true})
	if len(types.Symbols) != 1 || types.Symbols[0].Name != "Widget" || len(types.Symbols[0].Children) != 0 {
		t.Fatalf("types=%#v", types.Symbols)
	}
	both := FilterOutline(original, OutlineKinds{"types": true, "functions": true})
	if len(both.Symbols) != 2 || both.Symbols[0].Name != "Widget" || len(both.Symbols[0].Children) != 1 || both.Symbols[0].Children[0].Name != "Run" {
		t.Fatalf("types + functions=%#v", both.Symbols)
	}
	if len(original.Symbols[0].Children[0].Children) != 2 {
		t.Fatal("filter mutated original children")
	}
	if out := outlineOutput(types, "class Widget {}", true); strings.Contains(out, "class Widget {}") || !strings.Contains(out, "class\tWidget") {
		t.Fatalf("filtered output fell back to raw content: %q", out)
	}
}

func TestFilterOutlineIgnoresNonCodeShapeNodes(t *testing.T) {
	for _, test := range []struct{ path, source string }{
		{"settings.json", `{"nested": {"value": 1}}`},
		{"settings.yaml", "nested:\n  value: 1\n"},
		{"README.md", "# Overview\n"},
	} {
		outline := parser.OutlineFile(test.path, test.source)
		if len(outline.Symbols) == 0 {
			t.Fatalf("%s: test needs unfiltered shape nodes", test.path)
		}
		filtered := FilterOutline(outline, OutlineKinds{"types": true, "variables": true})
		if len(filtered.Symbols) != 0 {
			t.Fatalf("%s: shape nodes treated as code declarations: %#v", test.path, filtered.Symbols)
		}
	}
}

func TestOutlineCategoriesCoverCodeDeclarationKinds(t *testing.T) {
	for _, test := range []struct{ category, kinds string }{
		{"types", "type typedef class struct interface enum union trait record concept object"},
		{"functions", "func function fun method constructor"},
		{"variables", "var val const static field property"},
	} {
		for _, kind := range strings.Fields(test.kinds) {
			if got := symbolCategory(kind); got != test.category {
				t.Errorf("%s: got %q, want %q", kind, got, test.category)
			}
		}
	}
	for _, kind := range []string{"module", "namespace", "impl", "macro", "h1", "object-key"} {
		if got := symbolCategory(kind); got != "" {
			t.Errorf("%s: unexpected category %s", kind, got)
		}
	}
}
