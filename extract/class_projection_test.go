package extract

import (
	"strings"
	"testing"
)

func TestParseClassReadableProjectionFeatures(t *testing.T) {
	diagram := `classDiagram
 direction LR
 namespace domain_models {
  class User {
   +Name: string
  }
  <<struct>> User
  %% grepple:package User example.com/app/models
  class Account
  <<struct>> Account
  %% grepple:package Account example.com/app/models
 }
 User ..> Account : opens account
 Account <.. User : reverse dependency
`
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		t.Fatalf("ParseClassDiagram: %v", err)
	}
	if parsed.Direction != "LR" {
		t.Fatalf("Direction = %q, want LR", parsed.Direction)
	}
	for _, name := range []string{"User", "Account"} {
		if parsed.Classes[name] == nil {
			t.Fatalf("class %s was not retained with global identity", name)
		}
	}
	if len(parsed.Relations) != 2 {
		t.Fatalf("Relations = %+v", parsed.Relations)
	}
	first, second := parsed.Relations[0], parsed.Relations[1]
	if first.Operator != "..>" || first.Label != "opens account" {
		t.Fatalf("first relation = %+v", first)
	}
	if second.Operator != "<.." || second.Label != "reverse dependency" {
		t.Fatalf("second relation = %+v", second)
	}
}

func TestClassDiagramDirections(t *testing.T) {
	for _, direction := range []string{"LR", "RL", "TB", "BT"} {
		parsed, err := ParseClassDiagram("classDiagram\n direction " + direction + "\n")
		if err != nil || parsed.Direction != direction {
			t.Fatalf("direction %s: parsed=%+v err=%v", direction, parsed, err)
		}
	}
}

func TestClassProjectionSyntaxRejections(t *testing.T) {
	tests := []struct {
		name, diagram, want string
	}{
		{"malformed direction", "classDiagram\n direction TD\n", "malformed direction"},
		{"missing direction", "classDiagram\n direction\n", "malformed direction"},
		{"duplicate direction", "classDiagram\n direction LR\n direction BT\n", "duplicate direction"},
		{"direction in namespace", "classDiagram\n namespace n {\n direction LR\n }\n", "diagram top level"},
		{"malformed namespace", "classDiagram\n namespace bad-name {\n", "malformed namespace"},
		{"nested namespace", "classDiagram\n namespace one {\n namespace two {\n }\n }\n", "nested namespaces"},
		{"unclosed namespace", "classDiagram\n namespace one {\n class A\n", "Namespace 'one'"},
		{"unexpected close", "classDiagram\n }\n", "unexpected '}'"},
		{"relation in namespace", "classDiagram\n namespace one {\n class A\n class B\n A ..> B\n }\n", "relations are not allowed inside namespaces"},
		{"relation in class", "classDiagram\n class A {\n A ..> B\n }\n class B\n", "relations are not allowed inside class bodies"},
		{"duplicate global identity", "classDiagram\n namespace one {\n class A\n }\n namespace two {\n class A\n }\n", "duplicate diagram node 'A'"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseClassDiagram(test.diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDependencyRelationsCheckReferencesAndEndpoints(t *testing.T) {
	source := Source{"models.ts", `
class Account {}
class User { account: Account = new Account(); }
`}
	for _, relation := range []string{"User ..> Account : uses", "Account <.. User : uses"} {
		diagram := "classDiagram\n direction BT\n namespace models {\n class User\n class Account\n }\n " + relation + "\n"
		diagnostics, err := CheckClassDiagram(diagram, []Source{source})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("relation %q: err=%v diagnostics=%+v", relation, err, diagnostics)
		}
	}

	_, err := ParseClassDiagram("classDiagram\n class User\n User ..> Missing : uses\n")
	if err == nil || !strings.Contains(err.Error(), "relation endpoint 'Missing' is not declared") {
		t.Fatalf("undefined dependency endpoint error = %v", err)
	}
}

func TestClassDisplayLabelPreservesIdentity(t *testing.T) {
	diagram := `classDiagram
 class ID["ID = string"]
 <<type>> ID
 <<go>> ID
 %% grepple:package ID model
 %% grepple:underlying ID string
 class Alias["Alias = ID"]
 <<alias>> Alias
 <<go>> Alias
 %% grepple:package Alias model
 %% grepple:underlying Alias ID
 Alias ..> ID : underlying type
`
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Classes["Alias"] == nil || parsed.Classes["Alias"].DisplayLabel != "Alias = ID" {
		t.Fatalf("display label was not stored independently from ID: %+v", parsed.Classes["Alias"])
	}
	source := Source{"model.go", "package model\ntype ID string\ntype Alias = ID\n"}
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("display label changed validation identity: diagnostics=%+v err=%v", diagnostics, err)
	}
}
