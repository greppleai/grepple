package parser

import "testing"

func TestLuaNavigationAndOutline(t *testing.T) {
	content := "local function helper()\n  return 1\nend\nfunction run()\n  helper()\nend\n"
	outline := OutlineFile("app.lua", content)
	mustFind(t, outline.Symbols, "function", "helper")
	mustFind(t, outline.Symbols, "function", "run")
	graph := BuildNavigationGraph(content, "lua", "app.lua")
	if len(graph.Declarations) != 2 || len(graph.Calls) != 1 {
		t.Fatalf("Lua navigation: declarations=%+v calls=%+v", graph.Declarations, graph.Calls)
	}
	if graph.Calls[0].Name != "helper" || graph.Calls[0].CallerID != graph.Declarations[1].ID {
		t.Fatalf("Lua call: %+v", graph.Calls[0])
	}
	if len(graph.Imports) != 0 || len(graph.TypeUsages) != 0 {
		t.Fatalf("unexpected inferred Lua facts: %+v", graph)
	}
}
