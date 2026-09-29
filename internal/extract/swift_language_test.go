package extract

import (
	"path/filepath"
	"testing"
)

func TestSwiftFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "Sources", "App", "Main.swift"), Text: `public protocol Named { func greet(_ value: String) -> String }
class Base {}
struct Helper { func format(_ value: String) -> String { return value } }
class Runner: Base, Named {
  let helper: Helper
  init(helper: Helper) { self.helper = helper }
  func greet(_ value: String) -> String { return value }
  func run(_ value: String) -> String { finish(); return helper.format(value) }
  func finish() {}
}
func main() { let runner = Runner(helper: Helper()); print(runner.run("hi")) }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	module := absolutePath(source.Path)
	runner := analysis.ModuleDeclarations[module+":Runner"]
	if runner == nil || runner.Language != "swift" || !runner.Extends["Base"] || !runner.Implements["Named"] {
		t.Fatalf("Swift declaration=%#v", runner)
	}
	assertJVMMember(t, runner.Members, Member{Name: "helper", Kind: "property", Type: "Helper", Visibility: "package"})
	assertJVMMember(t, runner.Members, Member{Name: "run", Kind: "method", Type: "String", Visibility: "package"})
	if named := analysis.ModuleDeclarations[module+":Named"]; named == nil || named.Kind != "interface" {
		t.Fatalf("Swift protocol=%#v", named)
	}
	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Swift structure", structure, []string{"class Runner {", "class Base", "class Helper {", "<<swift>> Runner", "Base <|-- Runner", "~helper: Helper"})
	flow, err := GenerateFlowchart("Runner.run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Swift flow", flow, []string{"%% grepple:language Runner_run swift", "Runner_run --> Runner_finish"})
}

func TestSwiftExtensionDoesNotCreateDuplicateType(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "Sources", "Model.swift"), Text: "struct Model {}\nextension Model { func refresh() {} }\n"}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if model := analysis.ModuleDeclarations[absolutePath(source.Path)+":Model"]; model == nil || model.Kind != "struct" {
		t.Fatalf("Swift type=%#v", model)
	}
}

func TestSwiftFirstInheritedProtocolIsNotSuperclass(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "Sources", "Types.swift"), Text: `protocol Named {}
class Worker: Named {}
class Unknown: ExternalProtocol {}
class Derived: Base {}
class Base {}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	module := absolutePath(source.Path)
	worker := analysis.ModuleDeclarations[module+":Worker"]
	unknown := analysis.ModuleDeclarations[module+":Unknown"]
	derived := analysis.ModuleDeclarations[module+":Derived"]
	if worker == nil || worker.Extends["Named"] || !worker.Implements["Named"] || unknown == nil || len(unknown.Extends) != 0 || len(unknown.Implements) != 0 || derived == nil || !derived.Extends["Base"] {
		t.Fatalf("Swift inheritance: Worker=%+v Unknown=%+v Derived=%+v", worker, unknown, derived)
	}
}
