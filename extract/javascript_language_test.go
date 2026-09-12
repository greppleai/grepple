package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaScriptFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{
		Path: filepath.Join(root, "app.jsx"),
		Text: `export class Runner {
  start() { this.finish(); helper(); }
  finish() {}
}
export function helper() {}
const view = <Runner />;
`,
	}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.TSDeclarations[absolutePath(source.Path)+":Runner"]
	if declaration == nil || declaration.Language != "javascript" || len(declaration.Members) != 2 {
		t.Fatalf("declaration=%#v", declaration)
	}
	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"class Runner {", "<<javascript>> Runner", "%% grepple:module Runner app.jsx", "start()", "finish()"} {
		if !strings.Contains(structure, expected) {
			t.Fatalf("structure missing %q:\n%s", expected, structure)
		}
	}
	flow, err := GenerateFlowchart("Runner.start", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"%% grepple:language Runner_start javascript", "Runner_start --> Runner_finish", "Runner_start --> helper"} {
		if !strings.Contains(flow, expected) {
			t.Fatalf("flow missing %q:\n%s", expected, flow)
		}
	}
	if diagnostics, err := CheckClassDiagram(structure, []Source{source}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("structure diagnostics=%v err=%v", diagnostics, err)
	}
	if diagnostics, err := CheckFlowchart(flow, []Source{source}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("flow diagnostics=%v err=%v", diagnostics, err)
	}
}

func TestJavaScriptFocusedFlowResolvesNamedImport(t *testing.T) {
	root := t.TempDir()
	entry := Source{Path: filepath.Join(root, "app.js"), Text: `import { finish as done } from "./helper"; export function start() { done(); }`}
	helper := Source{Path: filepath.Join(root, "helper.js"), Text: `export function finish() {}`}
	flow, err := GenerateFlowchart("start", entry.Path, []Source{entry, helper}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"%% grepple:language start javascript", "%% grepple:language finish javascript", "start --> finish"} {
		if !strings.Contains(flow, expected) {
			t.Fatalf("flow missing %q:\n%s", expected, flow)
		}
	}
}
