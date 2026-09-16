package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRustFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "service.rs"), Text: `pub trait Task {
    type Error;
    fn execute(&self, value: String) -> Result<String, Self::Error>;
}

pub struct Helper;
impl Helper {
    pub fn run(&self, value: String) -> String { value }
}

pub struct Pair(pub u64, String);

pub struct Runner {
    pub helper: Helper,
    retries: usize,
    pub(crate) scoped: bool,
}

impl Runner {
    pub fn execute(&self, value: String) -> Result<String, String> {
        self.finish();
        Ok(self.helper.run(value))
    }
    fn finish(&self) {}
}

impl Task for Runner {
    type Error = String;
    fn execute(&self, value: String) -> Result<String, Self::Error> {
        Ok(self.helper.run(value))
    }
}

pub enum State<T> {
    Ready,
    Running(T),
    Failed { message: String },
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	runner := analysis.TSDeclarations[absolutePath(source.Path)+":Runner"]
	if runner == nil || runner.Language != "rust" || !runner.Implements["Task"] {
		t.Fatalf("Rust declaration=%#v", runner)
	}
	assertJVMMember(t, runner.Members, Member{Name: "helper", Kind: "property", Type: "Helper", Visibility: "public"})
	assertJVMMember(t, runner.Members, Member{Name: "retries", Kind: "property", Type: "usize", Visibility: "private"})
	assertJVMMember(t, runner.Members, Member{Name: "scoped", Kind: "property", Type: "bool", Visibility: "package"})
	assertJVMMember(t, runner.Members, Member{Name: "execute", Kind: "method", Type: "Result<String,String>", Visibility: "public"})
	assertJVMMember(t, runner.Members, Member{Name: "finish", Kind: "method", Visibility: "private"})
	task := analysis.TSDeclarations[absolutePath(source.Path)+":Task"]
	if task == nil || task.Kind != "interface" {
		t.Fatalf("Rust trait=%#v", task)
	}
	assertJVMMember(t, task.Members, Member{Name: "Error", Kind: "property", Type: "associated_type", Visibility: "public"})
	state := analysis.TSDeclarations[absolutePath(source.Path)+":State"]
	assertJVMMember(t, state.Members, Member{Name: "Ready", Kind: "property", Type: "State", Visibility: "public"})
	pair := analysis.TSDeclarations[absolutePath(source.Path)+":Pair"]
	assertJVMMember(t, pair.Members, Member{Name: "_0", Kind: "property", Type: "u64", Visibility: "public"})
	assertJVMMember(t, pair.Members, Member{Name: "_1", Kind: "property", Type: "String", Visibility: "private"})

	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Rust structure", structure, []string{"class Runner {", "class Helper {", "class Task {", "<<rust>> Runner", "<<interface>> Task", "Task <|.. Runner", "+helper: Helper", "~scoped: bool", "+execute(String): Result~String&#44;String~"})
	flow, err := GenerateFlowchart("Runner.execute", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Rust flow", flow, []string{"%% grepple:language Runner_execute rust", "Runner_execute --> Runner_finish", "Runner_execute --> Helper_run"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestRustFocusedExtractionMergesSameFileImplBlocks(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "model.rs"), Text: "pub struct Model { pub id: u64 }\nimpl Model { pub fn id(&self) -> u64 { self.id } }\nimpl Model { pub fn update(&mut self, id: u64) { self.id = id; } }\n"}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.TSDeclarations[absolutePath(source.Path)+":Model"]
	assertJVMMember(t, declaration.Members, Member{Name: "id", Kind: "method", Type: "u64", Visibility: "public"})
	assertJVMMember(t, declaration.Members, Member{Name: "update", Kind: "method", Visibility: "public"})
	structure, err := GenerateClassDiagram("Model", source, []Source{source}, GenerateOptions{Depth: 1, MaxNodes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(structure, "+id(): u64") || !strings.Contains(structure, "+update(u64)") {
		t.Fatalf("Rust impl methods missing from structure:\n%s", structure)
	}
}
