package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaFocusedFlow(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "app.lua"), Text: "local function helper()\n  return 1\nend\nfunction run()\n  helper()\nend\n"}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Symbols["helper"] == nil || analysis.Symbols["run"] == nil {
		t.Fatalf("missing Lua functions: %+v", analysis.Symbols)
	}
	flow, err := GenerateFlowchart("run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "%% grepple:language run lua") || !strings.Contains(flow, "run --> helper") {
		t.Fatalf("Lua flow missing helper:\n%s", flow)
	}
}
