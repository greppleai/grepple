package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoFunctionAndImportNodesAcceptExactPackageScope(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{filepath.Join(root, "worker", "worker.go"), "package worker\nimport helper \"example.com/dependency\"\nfunc Build() {}\nvar _ = helper.Value\n"}
	diagram := `classDiagram
 class Build {
  +Build()
 }
 <<function>> Build
 <<go>> Build
 %% grepple:package Build example.com/app/worker
 class helper
 <<import>> helper
 <<go>> helper
 %% grepple:package helper example.com/app/worker
 %% grepple:import helper from "example.com/dependency"
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("exact Go scope: %v %+v", err, diagnostics)
	}
}

func TestGoUnexportedInterfaceMethodsRequireSamePackage(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	contract := Source{filepath.Join(root, "contract", "contract.go"), "package contract\ntype private interface { run() }\ntype Public interface { Run() }\n"}
	implementation := Source{filepath.Join(root, "impl", "impl.go"), "package impl\ntype Worker struct{}\nfunc (Worker) run() {}\nfunc (Worker) Run() {}\n"}
	privateDiagram := goInterfaceRelationDiagram("private", "run", "example.com/app/contract")
	privateDiagnostics, err := CheckClassDiagram(privateDiagram, []Source{contract, implementation})
	if err != nil || len(privateDiagnostics) != 1 || !strings.Contains(privateDiagnostics[0].Message, "must implement") {
		t.Fatalf("private method crossed packages: %v %+v", err, privateDiagnostics)
	}
	publicDiagram := goInterfaceRelationDiagram("Public", "Run", "example.com/app/contract")
	publicDiagnostics, err := CheckClassDiagram(publicDiagram, []Source{contract, implementation})
	if err != nil || len(publicDiagnostics) != 0 {
		t.Fatalf("exported method should match across packages: %v %+v", err, publicDiagnostics)
	}
}

func goInterfaceRelationDiagram(contract, method, scope string) string {
	visibility := "-"
	if method == "Run" {
		visibility = "+"
	}
	return "classDiagram\n class " + contract + " {\n  " + visibility + method + "()\n }\n <<interface>> " + contract + "\n <<go>> " + contract + "\n %% grepple:package " + contract + " " + scope + "\n class Worker {\n  " + visibility + method + "()\n }\n <<struct>> Worker\n %% grepple:package Worker example.com/app/impl\n " + contract + " <|.. Worker\n"
}

func TestTypeScriptImportedAliasRelationsUseResolvedDeclaration(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "package.json"), `{}`)
	profile := Source{filepath.Join(root, "profile.ts"), "export interface Profile { value: string }\n"}
	holder := Source{filepath.Join(root, "holder.ts"), "import { Profile as P } from './profile'; export class Holder { profile: P }\nexport interface Extended extends P {}\n"}
	diagram, err := GenerateClassDiagram("Holder", holder, []Source{holder, profile}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagram, "Holder --> Profile") {
		t.Fatalf("alias association missing:\n%s", diagram)
	}
	if diagnostics, checkErr := CheckClassDiagram(diagram, []Source{holder, profile}); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("alias round trip: %v %+v", checkErr, diagnostics)
	}
	extended, err := GenerateClassDiagram("Extended", holder, []Source{holder, profile}, GenerateOptions{})
	if err != nil || !strings.Contains(extended, "Profile <|-- Extended") {
		t.Fatalf("alias extends relation: %v\n%s", err, extended)
	}
}

func TestTypeScriptExportAliasResolvesForClassAndFlow(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "package.json"), `{}`)
	library := Source{filepath.Join(root, "library.ts"), "class Profile {}\nfunction finish(): void {}\nexport { Profile as PublicProfile, finish as done };\n"}
	consumer := Source{filepath.Join(root, "consumer.ts"), "import { PublicProfile as P, done as complete } from './library'; export class Holder { profile: P }\nexport function start(): void { complete(); }\n"}
	sources := []Source{consumer, library}
	classDiagram, err := GenerateClassDiagram("Holder", consumer, sources, GenerateOptions{})
	if err != nil || !strings.Contains(classDiagram, "Holder --> Profile") {
		t.Fatalf("exported class alias: %v\n%s", err, classDiagram)
	}
	flowDiagram, err := GenerateFlowchart("start", consumer.Path, sources, GenerateOptions{})
	if err != nil || !strings.Contains(flowDiagram, "start --> finish") {
		t.Fatalf("exported function alias: %v\n%s", err, flowDiagram)
	}
}

func TestTypeScriptImportNodeHonorsImporterModuleMetadata(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "package.json"), `{}`)
	sources := []Source{
		{filepath.Join(root, "a.ts"), "import { X } from './one';\n"},
		{filepath.Join(root, "b.ts"), "import { X } from './two';\n"},
		{filepath.Join(root, "one.ts"), "export class X {}\n"},
		{filepath.Join(root, "two.ts"), "export class X {}\n"},
	}
	diagram := "classDiagram\n class X\n <<import>> X\n <<typescript>> X\n %% grepple:module X a.ts\n %% grepple:import X from './two'\n"
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Missing import") {
		t.Fatalf("unrelated import satisfied module scope: %+v", diagnostics)
	}
}
