package extract

import (
	"path/filepath"
	"testing"
)

func TestCompletePackageIncludesExpandedTypesAndExportedFunctions(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{Path: filepath.Join(root, "target", "a.go"), Text: `package target

type Alias = string
type ID string

func Build() {}
func hidden() {}
func init() {}
`},
		{Path: filepath.Join(root, "target", "z.go"), Text: `package target

type Worker struct{}
func (Worker) Method() {}
func Exported(value int) string { return "" }
`},
	}
	diagram := `classDiagram
 %% grepple:complete-package example.com/app/target
 class Worker
 <<struct>> Worker
 %% grepple:package Worker example.com/app/target
 class Build {
  +Build()
 }
 <<function>> Build
 <<go>> Build
 %% grepple:package Build example.com/app/target
`
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Package completeness missing alias 'Alias' declared at target/a.go:3.",
		"Package completeness missing type 'ID' declared at target/a.go:4.",
		"Package completeness missing function 'Exported' declared at target/z.go:5.",
	}
	if len(diagnostics) != len(want) {
		t.Fatalf("diagnostics: %+v", diagnostics)
	}
	for index := range want {
		if diagnostics[index].Line != 2 || diagnostics[index].Message != want[index] {
			t.Fatalf("diagnostic %d: got %+v, want %q", index, diagnostics[index], want[index])
		}
	}
}

func TestCompletePackageFunctionCoverageUsesExactPackageIdentity(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{Path: filepath.Join(root, "target", "build.go"), Text: "package target\nfunc Build() {}\n"},
		{Path: filepath.Join(root, "other", "build.go"), Text: "package other\nfunc Build() {}\n"},
	}
	diagram := `classDiagram
 %% grepple:complete-package example.com/app/target
 class Build {
  +Build()
 }
 <<function>> Build
 <<go>> Build
 %% grepple:package Build example.com/app/other
`
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "Package completeness missing function 'Build' declared at target/build.go:2." {
		t.Fatalf("package identity diagnostics: %+v", diagnostics)
	}
}
