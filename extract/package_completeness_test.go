package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func completenessSources(t *testing.T) (string, []Source) {
	t.Helper()
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	return root, []Source{
		{Path: filepath.Join(root, "target", "z.go"), Text: "package target\n\ntype Zed interface { Run() }\n"},
		{Path: filepath.Join(root, "target", "a.go"), Text: "package target\n\ntype (\n\tBeta struct{}\n\tAlpha interface{}\n)\n\nfunc f() {\n\ttype Local struct{}\n\t_ = Local{}\n}\n"},
	}
}

func TestCompletePackageReportsOnlyUncoveredPackageLevelGoTypes(t *testing.T) {
	_, sources := completenessSources(t)
	diagram := `classDiagram
 %% grepple:complete-package example.com/app/target
 class Beta
 <<struct>> Beta
 %% grepple:package Beta example.com/app/target
`
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	want := []string{
		"Package completeness missing interface 'Alpha' declared at target/a.go:5.",
		"Package completeness missing interface 'Zed' declared at target/z.go:3.",
	}
	for index := range want {
		if diagnostics[index].Line != 2 || diagnostics[index].Message != want[index] {
			t.Fatalf("diagnostic %d: got %+v, want line 2 %q", index, diagnostics[index], want[index])
		}
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, "Local") {
			t.Fatalf("function-local type was included: %+v", diagnostics)
		}
	}
}

func TestCompletePackageIsolatesSameNameDeclarationsByExactPackage(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{Path: filepath.Join(root, "target", "item.go"), Text: "package target\ntype Item struct{}\n"},
		{Path: filepath.Join(root, "other", "item.go"), Text: "package other\ntype Item struct{}\n"},
	}
	diagram := `classDiagram
 %% grepple:complete-package example.com/app/target
 class Item
 <<struct>> Item
 %% grepple:package Item example.com/app/other
`
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Line != 2 || !strings.Contains(diagnostics[0].Message, "struct 'Item'") || !strings.Contains(diagnostics[0].Message, "target/item.go:2") {
		t.Fatalf("package-isolated diagnostics: %+v", diagnostics)
	}
}

func TestCompletePackageOrderingUsesPathLineThenName(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{Path: filepath.Join(root, "pkg", "b.go"), Text: "package pkg\ntype First struct{}\n"},
		{Path: filepath.Join(root, "pkg", "a.go"), Text: "package pkg\n\n\n\n\n\n\n\n\n\ntype Tenth struct{}\ntype Second struct{}\n"},
	}
	diagram := "classDiagram\n %% pi:complete-package example.com/app/pkg\n"
	first, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || len(second) != len(first) {
		t.Fatalf("diagnostics: %+v", first)
	}
	want := []string{"pkg/a.go:11", "pkg/a.go:12", "pkg/b.go:2"}
	for index := range want {
		if first[index] != second[index] || !strings.Contains(first[index].Message, want[index]) {
			t.Fatalf("ordering at %d: first=%+v second=%+v", index, first, second)
		}
	}
}

func TestCompletePackageRejectsMalformedAndDuplicateDirectives(t *testing.T) {
	tests := []struct {
		name, diagram, want string
	}{
		{"missing path", "classDiagram\n %% grepple:complete-package\n", "malformed complete-package directive"},
		{"extra field", "classDiagram\n %% pi:complete-package example.com/app extra\n", "malformed complete-package directive"},
		{"duplicate prefixes", "classDiagram\n %% grepple:complete-package example.com/app\n %% pi:complete-package example.com/app\n", "Line 3: duplicate complete-package directive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseClassDiagram(test.diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}
