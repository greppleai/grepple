package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func fileLocalDiagram(name, scope string) string {
	return "classDiagram\n class " + name + "\n <<struct>> " + name + "\n <<go>> " + name + "\n %% grepple:package " + name + " " + scope + "\n %% grepple:filelocal " + name + "\n"
}

func TestGoFileLocalRequiresContiguousExactMarker(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	diagram := fileLocalDiagram("Node", "example.com/app")

	for _, test := range []struct {
		name, source string
	}{
		{"missing", "package app\ntype Node struct{}\n"},
		{"spaced marker", "package app\n// grepple:filelocal\ntype Node struct{}\n"},
		{"noncontiguous marker", "package app\n//grepple:filelocal\n\ntype Node struct{}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagnostics, err := CheckClassDiagram(diagram, []Source{{filepath.Join(root, "node.go"), test.source}})
			if err != nil || len(diagnostics) != 1 || diagnostics[0].Line != 6 || !strings.Contains(diagnostics[0].Message, "requires contiguous leading marker comment '//grepple:filelocal'") {
				t.Fatalf("marker diagnostic: %v %+v", err, diagnostics)
			}
		})
	}
}

func TestGoFileLocalAllowsUsesInDeclarationFile(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{filepath.Join(root, "node.go"), `package app
//grepple:filelocal
type Node struct { Next *Node }
func (*Node) Link(value Node) Node { return Node(value) }
var _ = Node{}
`}
	diagnostics, err := CheckClassDiagram(fileLocalDiagram("Node", "example.com/app"), []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("same-file uses: %v %+v", err, diagnostics)
	}
}

func TestGoFileLocalReportsFirstExternalStructuralUseDeterministically(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	declaration := Source{filepath.Join(root, "node.go"), "package app\n//grepple:filelocal\ntype Node struct{}\n"}
	first := Source{filepath.Join(root, "a_use.go"), "package app\nvar value Node\n"}
	second := Source{filepath.Join(root, "z_use.go"), "package app\nfunc makeNode() Node { return Node{} }\n"}
	diagram := fileLocalDiagram("Node", "example.com/app")

	for _, sources := range [][]Source{{second, declaration, first}, {first, second, declaration}} {
		diagnostics, err := CheckClassDiagram(diagram, sources)
		if err != nil || len(diagnostics) != 1 {
			t.Fatalf("external use: %v %+v", err, diagnostics)
		}
		if diagnostics[0].Line != 6 || !strings.Contains(diagnostics[0].Message, "a_use.go:2:11") {
			t.Fatalf("first external use was not deterministic: %+v", diagnostics)
		}
	}
}

func TestGoFileLocalCatchesExternalConversion(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{filepath.Join(root, "node.go"), "package app\n//grepple:filelocal\ntype Node struct{}\n"},
		{filepath.Join(root, "convert.go"), "package app\nfunc convert(value any) { _ = Node(value) }\n"},
	}
	diagnostics, err := CheckClassDiagram(fileLocalDiagram("Node", "example.com/app"), sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "convert.go:2:31") {
		t.Fatalf("conversion use: %v %+v", err, diagnostics)
	}
}

func TestGoFileLocalUsesExactPackageIdentity(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{filepath.Join(root, "one", "node.go"), "package shared\n//grepple:filelocal\ntype Node struct{}\n"},
		{filepath.Join(root, "two", "use.go"), "package shared\nvar value Node\n"},
	}
	diagnostics, err := CheckClassDiagram(fileLocalDiagram("Node", "example.com/app/one"), sources)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("separate package was treated as the same package: %v %+v", err, diagnostics)
	}
}

func TestGeneratedGoFileLocalMetadataAndLegacyPrefix(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{filepath.Join(root, "node.go"), "package app\n//grepple:filelocal\ntype Node struct{}\n"}
	generated, err := GenerateClassDiagram("Node", source, []Source{source}, GenerateOptions{})
	if err != nil || !strings.Contains(generated, "%% grepple:filelocal Node") {
		t.Fatalf("generated metadata: %v\n%s", err, generated)
	}
	legacy := strings.Replace(fileLocalDiagram("Node", "example.com/app"), "%% grepple:filelocal", "%% pi:filelocal", 1)
	if diagnostics, checkErr := CheckClassDiagram(legacy, []Source{source}); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("legacy metadata: %v %+v", checkErr, diagnostics)
	}
}
