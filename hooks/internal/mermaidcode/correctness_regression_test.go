package mermaidcode

import (
	"strings"
	"testing"
)

func TestStructStereotypeImpliesGoAndRejectsTypeScript(t *testing.T) {
	diagram, err := ParseClassDiagram("classDiagram\n class Record\n <<struct>> Record\n")
	if err != nil {
		t.Fatal(err)
	}
	if diagram.Classes["Record"].Kind != "struct" || diagram.Classes["Record"].Language != "go" {
		t.Fatalf("struct stereotype did not imply Go: %+v", diagram.Classes["Record"])
	}
	for _, source := range []string{
		"classDiagram\n class Record\n <<struct>> Record\n <<typescript>> Record\n",
		"classDiagram\n class Record\n <<typescript>> Record\n <<struct>> Record\n",
		"classDiagram\n class Record\n <<interface>> Record\n <<struct>> Record\n",
		"classDiagram\n class Record\n <<struct>> Record\n <<function>> Record\n",
	} {
		if _, parseErr := ParseClassDiagram(source); parseErr == nil || !strings.Contains(parseErr.Error(), "conflicts") {
			t.Fatalf("expected stereotype conflict, got %v", parseErr)
		}
	}
}

func TestTypeNormalizationIsLanguageSpecific(t *testing.T) {
	goSource := Source{"numbers.go", `package numbers

type Numbers struct { Count int; Ratio float }
func (n *Numbers) Add(values ...int) int { return len(values) }
`}
	goDiagram := `classDiagram
 class Numbers {
  +Count: int
  +Ratio: float
  +Add(...int): int
 }
 <<struct>> Numbers
`
	diagnostics, err := CheckClassDiagram(goDiagram, []Source{goSource})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("Go normalization: %v, %+v", err, diagnostics)
	}
	generated, err := GenerateClassDiagram("Numbers", goSource, []Source{goSource}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"+Count: int", "+Ratio: float", "+Add(...int): int"} {
		if !strings.Contains(generated, expected) {
			t.Errorf("missing %q in %s", expected, generated)
		}
	}

	tsDiagram := "classDiagram\n class Values {\n  +int count\n }\n"
	tsSource := Source{"values.ts", "class Values { count: number = 1; }"}
	diagnostics, err = CheckClassDiagram(tsDiagram, []Source{tsSource})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("TypeScript aliases regressed: %v, %+v", err, diagnostics)
	}
}

func TestUnqualifiedMixedDeclarationsAndFunctionsAreAmbiguous(t *testing.T) {
	sources := []Source{
		{"same.go", "package same\ntype Same struct{}\nfunc Work() {}\n"},
		{"same.ts", "class Same {}\nfunction Work(): void {}\n"},
	}
	classDiagram := `classDiagram
 class Same
 class Work {
  +Work()
 }
 <<function>> Work
`
	diagnostics, err := CheckClassDiagram(classDiagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	messages := diagnosticsText(diagnostics)
	if !strings.Contains(messages, "Ambiguous code declaration 'Same'") || !strings.Contains(messages, "Ambiguous code function 'Work'") {
		t.Fatalf("missing ambiguity diagnostics: %s", messages)
	}

	flow := "flowchart TD\n Work[\"Work\"]\n"
	diagnostics, err = CheckFlowchart(flow, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous code symbol 'Work'") {
		t.Fatalf("unexpected flow ambiguity diagnostics: %+v", diagnostics)
	}
}

func TestMalformedEmptyMethodNameReturnsError(t *testing.T) {
	_, err := ParseClassDiagram("classDiagram\n class Broken {\n  +()\n }\n")
	if err == nil || !strings.Contains(err.Error(), "method must include a name") {
		t.Fatalf("got %v, want method-name parse error", err)
	}
}
