package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPythonFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	base := Source{Path: filepath.Join(root, "models.py"), Text: `class Base:
    def close(self) -> None:
        pass
`}
	entry := Source{Path: filepath.Join(root, "app.py"), Text: `class Runner(Base):
    count: int = 0

    def __init__(self, dependency: Base):
        self.dependency: Base = dependency

    async def start(self, value: str) -> bool:
        self.finish()
        helper()
        return True

    def finish(self) -> None:
        pass

def helper() -> None:
    pass
`}
	sources := []Source{entry, base}
	analysis, err := Analyze(sources)
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.ModuleDeclarations[absolutePath(entry.Path)+":Runner"]
	if declaration == nil || declaration.Language != "python" || !declaration.Extends["Base"] {
		t.Fatalf("declaration=%#v", declaration)
	}
	if analysis.Symbols["Runner"] == nil || analysis.ModuleSymbols[absolutePath(entry.Path)+":Runner.start"] == nil {
		t.Fatalf("Python method symbols missing: %#v", sortedKeys(analysis.ModuleSymbols))
	}
	assertPythonMembers(t, declaration.Members, []Member{
		{Name: "count", Kind: "property", Type: "number"},
		{Name: "dependency", Kind: "property", Type: "Base"},
		{Name: "start", Kind: "method", Type: "bool"},
		{Name: "finish", Kind: "method", Type: "None"},
	})

	structure, err := GenerateClassDiagram("Runner", entry, sources, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "structure", structure, []string{"class Runner {", "class Base {", "<<python>> Runner", "Base <|-- Runner", "count: number", "dependency: Base", "async start(str): bool"})
	flow, err := GenerateFlowchart("Runner.start", entry.Path, sources, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "flow", flow, []string{"%% grepple:language Runner_start python", "Runner_start --> Runner_finish", "Runner_start --> helper"})
	if diagnostics, checkErr := CheckClassDiagram(structure, sources); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("structure diagnostics=%v err=%v", diagnostics, checkErr)
	}
	if diagnostics, checkErr := CheckFlowchart(flow, sources); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("flow diagnostics=%v err=%v", diagnostics, checkErr)
	}
}

func TestPythonFocusedExtractionSupportsDecoratorsAndStubFiles(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "service.pyi"), Text: `class Service:
    @staticmethod
    def create(value: str) -> "Service": ...

    @property
    def name(self) -> str: ...
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.ModuleDeclarations[absolutePath(source.Path)+":Service"]
	if declaration == nil || len(declaration.Members) != 2 {
		t.Fatalf("declaration=%#v", declaration)
	}
	if !declaration.Members[0].Static && !declaration.Members[1].Static {
		t.Fatalf("staticmethod not retained: %#v", declaration.Members)
	}
	if !hasPythonMember(declaration.Members, "name", "property", "str") {
		t.Fatalf("property decorator not retained: %#v", declaration.Members)
	}
	structure, err := GenerateClassDiagram("Service", source, []Source{source}, GenerateOptions{Depth: 1, MaxNodes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(structure, "%% grepple:module Service service.pyi") {
		t.Fatalf("stub module metadata missing:\n%s", structure)
	}
}

func TestPythonFocusedFlowUsesImportedTargetDespiteUnrelatedDuplicate(t *testing.T) {
	root := t.TempDir()
	entry := Source{Path: filepath.Join(root, "app.py"), Text: "from helper import finish\n\ndef start():\n    finish()\n"}
	helper := Source{Path: filepath.Join(root, "helper.py"), Text: "def finish():\n    pass\n"}
	flow, err := GenerateFlowchart("start", entry.Path, []Source{entry, helper}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "start --> finish") {
		t.Fatalf("unique cross-file call was not resolved:\n%s", flow)
	}
	analysis, err := Analyze([]Source{entry, helper})
	if err != nil {
		t.Fatal(err)
	}
	var confidence string
	for _, call := range analysis.Navigation.Calls {
		if call.Name == "finish" {
			confidence = call.Confidence
		}
	}
	if confidence != "import-resolved" {
		t.Fatalf("cross-file confidence = %q", confidence)
	}

	duplicate := Source{Path: filepath.Join(root, "other.py"), Text: "def finish():\n    pass\n"}
	resolved, err := GenerateFlowchart("start", entry.Path, []Source{entry, helper, duplicate}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolved, "start --> finish") {
		t.Fatalf("imported cross-file call was not retained:\n%s", resolved)
	}
}

func assertPythonMembers(t *testing.T, actual, expected []Member) {
	t.Helper()
	for _, member := range expected {
		if !hasPythonMember(actual, member.Name, member.Kind, member.Type) {
			t.Fatalf("Python declaration missing %#v: %#v", member, actual)
		}
	}
}

func assertContainsAll(t *testing.T, label, content string, expected []string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("%s missing %q:\n%s", label, value, content)
		}
	}
}

func hasPythonMember(members []Member, name, kind, typeName string) bool {
	for _, member := range members {
		if member.Name == name && member.Kind == kind && member.Type == typeName {
			return true
		}
	}
	return false
}
