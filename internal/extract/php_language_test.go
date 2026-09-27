package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPHPFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "service.php"), Text: `<!doctype html>
<?php
namespace App\Service;
class Base {}
class Formatter extends Base {
    public string $label;
    public function format(string $value): string { return helper($value); }
    private function unused(): void {}
}
function helper(string $value): string { return strtoupper($value); }
?>
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.ModuleDeclarations[absolutePath(source.Path)+":Formatter"]
	if declaration == nil {
		t.Fatalf("missing PHP class in %v", analysis.ModuleDeclarations)
	}
	if len(declaration.Members) < 3 {
		t.Fatalf("PHP members=%#v", declaration.Members)
	}
	if !declaration.Extends["Base"] {
		t.Fatalf("PHP base class=%#v", declaration.Extends)
	}
	assertJVMMember(t, declaration.Members, Member{Name: "label", Kind: "property", Type: "string", Visibility: "public"})
	assertJVMMember(t, declaration.Members, Member{Name: "format", Kind: "method", Type: "string", Visibility: "public", Parameters: []string{"string"}})
	structure, err := GenerateClassDiagram("Formatter", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(structure, "class Formatter") || !strings.Contains(structure, "format") {
		t.Fatalf("PHP structure=%s", structure)
	}
	flow, err := GenerateFlowchart("Formatter.format", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "helper") {
		t.Fatalf("PHP flow=%s", flow)
	}
}
