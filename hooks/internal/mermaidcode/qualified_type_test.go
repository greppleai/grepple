package mermaidcode

import (
	"strings"
	"testing"
)

func TestQualifiedTypesDoNotReferenceUnqualifiedLocalTypes(t *testing.T) {
	tests := []struct{ name, language, source, kind string }{
		{"go", "go", "package q\ntype User struct{}\ntype Holder struct { User external.User }\n", "struct"},
		{"typescript", "typescript", "class User {}\nclass Holder { user: Namespace.User; }\n", "class"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := ""
			if test.kind == "struct" {
				kind = " <<struct>> User\n <<struct>> Holder\n"
			} else {
				kind = " <<typescript>> Holder\n"
			}
			diagram := "classDiagram\n class User\n class Holder\n" + kind + " Holder --> User\n"
			diagnostics, err := CheckClassDiagram(diagram, []Source{{"qualified." + map[string]string{"go": "go", "typescript": "ts"}[test.language], test.source}})
			if err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) == 0 || !strings.Contains(diagnostics[len(diagnostics)-1].Message, "must contain a reference") {
				t.Fatalf("qualified type satisfied local relation: %+v", diagnostics)
			}
		})
	}
}
