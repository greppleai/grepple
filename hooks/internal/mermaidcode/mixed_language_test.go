package mermaidcode

import (
	"strings"
	"testing"
)

func TestLanguageStereotypesDisambiguateMixedRoots(t *testing.T) {
	sources := []Source{
		{"shared.go", `package mixed

type Shared struct { GoField string }
func (s *Shared) GoMethod(value int) int { return value }
func Work(value string) error { return nil }
`},
		{"shared.ts", `class Shared { tsField: number = 1; }
function Work(value: number): void {}
`},
	}
	goDiagram := `classDiagram
 class Shared {
  +GoField: string
  +GoMethod(int): int
 }
 <<struct>> Shared
 <<go>> Shared
 class Work {
  +Work(string): error
 }
 <<function>> Work
 <<go>> Work
`
	tsDiagram := `classDiagram
 class Shared {
  +tsField: number
 }
 <<typescript>> Shared
 class Work {
  +Work(number): void
 }
 <<function>> Work
 <<typescript>> Work
`
	for name, diagram := range map[string]string{"go": goDiagram, "typescript": tsDiagram} {
		diagnostics, err := CheckClassDiagram(diagram, sources)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(diagnostics) != 0 {
			t.Fatalf("%s diagnostics: %+v", name, diagnostics)
		}
	}
}

func TestFlowLanguageMetadataDisambiguatesMixedRoots(t *testing.T) {
	sources := []Source{
		{"flow.go", `package mixed
func start() { finish() }
func finish() {}
`},
		{"flow.ts", `function start(): void {}
function finish(): void { start(); }
`},
	}
	flow := `flowchart TD
 start["start"]
 finish["finish"]
 %% grepple:language start go
 %% grepple:language finish go
 start --> finish
`
	diagnostics, err := CheckFlowchart(flow, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestOmittedFlowMetadataDoesNotCrossLanguages(t *testing.T) {
	sources := []Source{
		{"start.go", "package mixed\nfunc goStart() { tsFinish() }\n"},
		{"finish.ts", "function tsFinish(): void {}\n"},
	}
	flow := "flowchart TD\n goStart[\"goStart\"]\n tsFinish[\"tsFinish\"]\n goStart --> tsFinish\n"
	diagnostics, err := CheckFlowchart(flow, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "No static call") {
		t.Fatalf("cross-language edge was not rejected: %+v", diagnostics)
	}
}
