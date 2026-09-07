package mermaidcode

import (
	"strings"
	"testing"
)

func TestClassParameterParserIgnoresNestedTypeColons(t *testing.T) {
	diagram := `classDiagram
class Service {
 +objectValue({ key: string, nested: { count: number } }): void
 +callback((value: string) => void): void
 +tuple([name: string, count: number]): void
 +named(options: { key: string }): void
}
`
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		t.Fatal(err)
	}
	members := parsed.Classes["Service"].Members
	want := []string{"{key:string,nested:{count:number}}", "(value:string)=>void", "[name:string,count:number]", "{key:string}"}
	for index, member := range members {
		if len(member.Parameters) != 1 || member.Parameters[0] != want[index] {
			t.Fatalf("member %d parameters = %#v, want %q", index, member.Parameters, want[index])
		}
	}
}

func TestTypeScriptGenerationRoundTripsNestedParameterTypes(t *testing.T) {
	source := Source{Path: "service.ts", Text: `export class Service {
  process(objectValue: { key: string; nested: { count: number } }, callback: (value: string) => void, tuple: [name: string, count: number], nested: Promise<{ key: string }>): void {}
}`}
	diagram, err := GenerateClassDiagram("Service", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateClassDiagram: %v", err)
	}
	for _, fragment := range []string{"key:string", "value:string", "name:string", "count:number"} {
		if !strings.Contains(diagram, fragment) {
			t.Fatalf("generated diagram missing nested type fragment %q:\n%s", fragment, diagram)
		}
	}
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("generated diagram did not round trip: %v %+v\n%s", err, diagnostics, diagram)
	}
}
