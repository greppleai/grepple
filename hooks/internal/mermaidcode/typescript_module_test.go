package mermaidcode

import (
	"strings"
	"testing"
)

func TestTypeScriptModuleIsolationAndEntryGeneration(t *testing.T) {
	first := Source{"/project/one.ts", `class Shared { one: string = ""; }
class Service { run(): void {} }
function start(): void { finish(); }
`}
	second := Source{"/project/two.ts", `class Shared { two: number = 2; }
class Service { run(): void {} }
function start(): void {}
function finish(): void {}
`}
	sources := []Source{first, second}
	classDiagram := "classDiagram\n class Shared\n <<typescript>> Shared\n"
	diagnostics, err := CheckClassDiagram(classDiagram, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous code declaration") {
		t.Fatalf("declaration ambiguity: %v %+v", err, diagnostics)
	}
	functionDiagram := "classDiagram\n class start {\n  +start(): void\n }\n <<function>> start\n <<typescript>> start\n"
	diagnostics, err = CheckClassDiagram(functionDiagram, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous code function") {
		t.Fatalf("function ambiguity: %v %+v", err, diagnostics)
	}
	flow := "flowchart TD\n run[\"run\"]\n %% grepple:symbol run Service.run\n %% grepple:language run typescript\n"
	diagnostics, err = CheckFlowchart(flow, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous code symbol") {
		t.Fatalf("method ambiguity: %v %+v", err, diagnostics)
	}
	generated, err := GenerateClassDiagram("Shared", first, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "+one: string") || strings.Contains(generated, "+two: number") {
		t.Fatalf("wrong entry declaration generated: %s", generated)
	}
}

func TestTypeScriptBareCallsDoNotCrossModules(t *testing.T) {
	sources := []Source{
		{"/project/start.ts", "function start(): void { finish(); }\n"},
		{"/project/finish.ts", "function finish(): void {}\n"},
	}
	flow := "flowchart TD\n start[\"start\"]\n finish[\"finish\"]\n start --> finish\n"
	diagnostics, err := CheckFlowchart(flow, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "No static call") {
		t.Fatalf("false cross-module edge accepted: %+v", diagnostics)
	}
	generated, err := GenerateFlowchart("start", sources[0].Path, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated, "finish[") || strings.Contains(generated, "start --> finish") {
		t.Fatalf("flow generation crossed modules: %s", generated)
	}
}
