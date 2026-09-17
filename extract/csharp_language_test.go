package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCSharpFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "app.csproj"), `<Project Sdk="Microsoft.NET.Sdk" />`)
	source := Source{Path: filepath.Join(root, "Worker.cs"), Text: `
public class BaseWorker {}
public interface IWorker { void Run(int count); }
public class Worker : BaseWorker, IWorker {
    public string Name { get; set; }
    private int count;
    public Worker(string name) { Name = name; }
    public static Worker Create() { return new Worker("x"); }
    public void Run(int count) { Save(); }
    private void Save() {}
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.ModuleDeclarations[absolutePath(source.Path)+":Worker"]
	if declaration == nil || declaration.Language != "csharp" {
		t.Fatalf("declaration=%#v", declaration)
	}
	for _, name := range []string{"Name", "count", "Worker", "Create", "Run", "Save"} {
		if !hasMemberNamed(declaration.Members, name) {
			t.Fatalf("missing member %q: %#v", name, declaration.Members)
		}
	}
	structure, err := GenerateClassDiagram("Worker", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"class Worker {", "<<csharp>> Worker", "BaseWorker <|-- Worker", "IWorker <|.. Worker", "+Run(number)"} {
		if !strings.Contains(structure, expected) {
			t.Fatalf("structure missing %q:\n%s", expected, structure)
		}
	}
	flow, err := GenerateFlowchart("Worker.Run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "Worker_Run --> Worker_Save") || !strings.Contains(flow, "%% grepple:language Worker_Run csharp") {
		t.Fatalf("unexpected flow:\n%s", flow)
	}
}

func TestCSharpProjectRootAndLanguageRegistration(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "app.sln"), "")
	nested := filepath.Join(root, "src", "feature")
	if got := nearestCSharpRoot(nested); got != root {
		t.Fatalf("root=%q, want %q", got, root)
	}
	language, ok := LanguageForPath("Worker.cs")
	if !ok || language.ID != "csharp" || !language.FocusedStructure || !language.FocusedFlow {
		t.Fatalf("language=%#v ok=%v", language, ok)
	}
}

func hasMemberNamed(members []Member, name string) bool {
	for _, member := range members {
		if member.Name == name {
			return true
		}
	}
	return false
}
