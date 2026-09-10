package extract

import (
	"strings"
	"testing"
)

func TestExactStereotypeRejectsAdditionalMembers(t *testing.T) {
	source := Source{"controller.go", `package controller

type healthController struct {
	registry string
	root string
}
`}
	diagram := `classDiagram
 class healthController {
  -root: string
 }
 <<struct>> healthController
 <<exact>> healthController
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Unexpected private registry: string on exact 'healthController'.") {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestSubsetDiagramStillAllowsAdditionalMembers(t *testing.T) {
	source := Source{"item.ts", `class Item { id: string = ""; extra: number = 1 }`}
	diagram := `classDiagram
 class Item {
  +id: string
 }
 <<typescript>> Item
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("subset validation changed: %v %+v", err, diagnostics)
	}
}
