package extract

import (
	"path/filepath"
	"testing"
)

func TestPHPFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "greeter.php"), Text: `<?php
class Greeter {
    private string $prefix;
    public function run(string $value): string { return $this->format($value); }
    private function format(string $value): string { return $value; }
}
function greet(): string { return helper(); }
function helper(): string { return "hi"; }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	greeter := analysis.ModuleDeclarations[absolutePath(source.Path)+":Greeter"]
	if greeter == nil {
		t.Fatalf("PHP declaration missing: %#v", analysis.ModuleDeclarations)
	}
	assertJVMMember(t, greeter.Members, Member{Name: "prefix", Kind: "property", Type: "string", Visibility: "private"})
	assertJVMMember(t, greeter.Members, Member{Name: "run", Kind: "method", Type: "string", Visibility: "public", Parameters: []string{"string"}})
	structure, err := GenerateClassDiagram("Greeter", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "PHP structure", structure, []string{"<<php>> Greeter", "-prefix: string", "+run(string): string"})
	flow, err := GenerateFlowchart("greet", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "PHP flow", flow, []string{"%% grepple:language greet php", "greet --> helper"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestPHPBracedNamespaceAndHeritage(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "types.php"), Text: `<?php
namespace App\Domain {
 interface Named { public function name(): string; }
 class Base {}
 class Child extends Base implements Named {
  public function name(): string { return "child"; }
 }
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	child := analysis.ModuleDeclarations[absolutePath(source.Path)+":Child"]
	if child == nil || child.Package != `App\Domain` || !child.Extends["Base"] || !child.Implements["Named"] {
		t.Fatalf("PHP namespace/heritage: %#v", child)
	}
}
