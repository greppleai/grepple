package mermaidcode

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoAliasesAndNamedTypesAreDeclarations(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{Path: filepath.Join(root, "model", "model.go"), Text: `package model

type Alias = string
type ID string
type Records []ID

func (ID) String() string { return "" }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	for name, kind := range map[string]string{"Alias": "alias", "ID": "type", "Records": "type"} {
		declaration := analysis.GoDeclarations[filepath.Clean(root+"/model")+":model:"+name]
		if declaration == nil || declaration.Kind != kind || declaration.PackageID == "" {
			t.Fatalf("%s declaration: %+v", name, declaration)
		}
	}
	if declaration := analysis.Declarations["ID"]; declaration == nil || len(declaration.Members) != 1 || declaration.Members[0].Name != "String" {
		t.Fatalf("named type methods were not retained: %+v", declaration)
	}

	diagram := `classDiagram
 class Alias
 <<alias>> Alias
 %% grepple:package Alias example.com/app/model
 class ID {
  +String(): string
 }
 <<type>> ID
 %% grepple:package ID example.com/app/model
 class Records
 <<type>> Records
 %% grepple:package Records example.com/app/model
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("validation: %v %+v", err, diagnostics)
	}
}

func TestAliasAndTypeStereotypeConflicts(t *testing.T) {
	cases := []string{
		"<<alias>> Node\n<<type>> Node",
		"<<type>> Node\n<<struct>> Node",
		"<<interface>> Node\n<<alias>> Node",
		"<<typescript>> Node\n<<type>> Node",
		"<<alias>> Node\n<<function>> Node",
	}
	for _, stereotypes := range cases {
		_, err := ParseClassDiagram("classDiagram\nclass Node\n" + stereotypes + "\n")
		if err == nil || !strings.Contains(err.Error(), "conflicts with the kind or language") {
			t.Errorf("stereotypes %q: got %v", stereotypes, err)
		}
	}
}

func TestGoAliasAndNamedTypeGenerationRoundTrips(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{Path: filepath.Join(root, "model", "model.go"), Text: "package model\n\ntype Alias = string\ntype ID string\nfunc (ID) String() string { return \"\" }\n"}
	for name, stereotype := range map[string]string{"Alias": "<<alias>> Alias", "ID": "<<type>> ID"} {
		generated, err := GenerateClassDiagram(name, source, []Source{source}, GenerateOptions{})
		if err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
		if !strings.Contains(generated, stereotype) || !strings.Contains(generated, "%% grepple:package "+name+" example.com/app/model") {
			t.Fatalf("generated %s did not retain kind/package:\n%s", name, generated)
		}
		parsed, err := ParseClassDiagram(generated)
		if err != nil || parsed.Classes[name].Kind != map[string]string{"Alias": "alias", "ID": "type"}[name] {
			t.Fatalf("round trip %s: %v %+v", name, err, parsed.Classes[name])
		}
	}
}
