package mermaidcode

import (
	"strings"
	"testing"
)

func TestClassRelationsRequireDeclaredEndpoints(t *testing.T) {
	tests := []struct {
		name, diagram, endpoint string
	}{
		{
			name:     "undefined owner",
			diagram:  "classDiagram\n Missing --> Target\n class Target\n",
			endpoint: "Missing",
		},
		{
			name:     "undefined target",
			diagram:  "classDiagram\n class Owner\n Owner --> Missing\n",
			endpoint: "Missing",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseClassDiagram(test.diagram)
			if err == nil || !strings.Contains(err.Error(), "relation endpoint '"+test.endpoint+"' is not declared") {
				t.Fatalf("expected undefined endpoint error, got %v", err)
			}
		})
	}
}

func TestFunctionAssociationsValidateParametersAndReturn(t *testing.T) {
	source := Source{"relations.ts", `
class Input {}
class Output {}
class Unrelated {}
class Holder { input: Input = new Input(); }
function transform(input: Input): Output { return new Output(); }
`}
	diagram := `classDiagram
 class Input
 class Output
 class Holder
 class transform {
  +transform(Input): Output
 }
 <<function>> transform
 transform --> Input
 transform --> Output
 Holder --> Input
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("valid function and type associations: %v %+v", err, diagnostics)
	}

	invalid := strings.Replace(diagram, " class Output\n", " class Output\n class Unrelated\n", 1)
	invalid = strings.Replace(invalid, " transform --> Output\n", " transform --> Unrelated\n", 1)
	diagnostics, err = CheckClassDiagram(invalid, []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "'transform' must contain a reference to 'Unrelated'") {
		t.Fatalf("invalid function association was not rejected: %+v", diagnostics)
	}
}
