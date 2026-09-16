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

func TestRustFocusedExtractionKeepsInlineModuleOwnership(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "src", "lib.rs"), Text: `mod first {
	pub struct Model;
	impl Model { pub fn first(&self) {} }
}
mod second {
	pub struct Model;
	impl Model { pub fn second(&self) {} }
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	first := analysis.TSDeclarations[absolutePath(source.Path)+":first::Model"]
	second := analysis.TSDeclarations[absolutePath(source.Path)+":second::Model"]
	if first == nil || second == nil {
		t.Fatalf("inline Rust declarations=%#v", analysis.TSDeclarations)
	}
	assertJVMMember(t, first.Members, Member{Name: "first", Kind: "method", Visibility: "public"})
	assertJVMMember(t, second.Members, Member{Name: "second", Kind: "method", Visibility: "public"})
	for _, member := range first.Members {
		if member.Name == "second" {
			t.Fatalf("second module impl attached to first: %#v", first.Members)
		}
	}
}

func TestRustFocusedExtractionAttachesSyntaxResolvedCrossFileImpls(t *testing.T) {
	root := t.TempDir()
	lib := Source{Path: filepath.Join(root, "src", "lib.rs"), Text: "mod model; mod impls;\n"}
	model := Source{Path: filepath.Join(root, "src", "model.rs"), Text: "pub struct Model;\n"}
	impls := Source{Path: filepath.Join(root, "src", "impls.rs"), Text: `use crate::model::Model;
impl Model { pub fn imported(&self) {} }
impl crate::model::Model { pub fn qualified(&self) {} }
`}
	analysis, err := Analyze([]Source{lib, model, impls})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.TSDeclarations[absolutePath(model.Path)+":Model"]
	if declaration == nil {
		t.Fatalf("Rust model declarations=%#v", analysis.TSDeclarations)
	}
	assertJVMMember(t, declaration.Members, Member{Name: "imported", Kind: "method", Visibility: "public"})
	assertJVMMember(t, declaration.Members, Member{Name: "qualified", Kind: "method", Visibility: "public"})
	structure, err := GenerateClassDiagram("Model", model, []Source{lib, model, impls}, GenerateOptions{Depth: 1, MaxNodes: 10})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "cross-file Rust impl structure", structure, []string{"+imported()", "+qualified()"})
}

func TestRustFocusedExtractionLeavesAmbiguousCrossFileImplUnattached(t *testing.T) {
	root := t.TempDir()
	lib := Source{Path: filepath.Join(root, "src", "lib.rs"), Text: "mod model; mod impls;\n"}
	flat := Source{Path: filepath.Join(root, "src", "model.rs"), Text: "pub struct Model;\n"}
	directory := Source{Path: filepath.Join(root, "src", "model", "mod.rs"), Text: "pub struct Model;\n"}
	impls := Source{Path: filepath.Join(root, "src", "impls.rs"), Text: "use crate::model::Model;\nimpl Model { pub fn ambiguous(&self) {} }\n"}
	analysis, err := Analyze([]Source{lib, flat, directory, impls})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []Source{flat, directory} {
		declaration := analysis.TSDeclarations[absolutePath(source.Path)+":Model"]
		if declaration == nil {
			t.Fatalf("missing Rust model for %s", source.Path)
		}
		for _, member := range declaration.Members {
			if member.Name == "ambiguous" {
				t.Fatalf("ambiguous impl attached to %s: %#v", source.Path, declaration.Members)
			}
		}
	}
}

func TestRustFocusedExtractionDoesNotFallbackForUnresolvedQualifiedImpl(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "src", "lib.rs"), Text: "pub struct Model;\nimpl external::Model { pub fn wrong(&self) {} }\n"}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.TSDeclarations[absolutePath(source.Path)+":Model"]
	if declaration == nil {
		t.Fatal("missing local Rust model")
	}
	for _, member := range declaration.Members {
		if member.Name == "wrong" {
			t.Fatalf("unresolved qualified impl attached locally: %#v", declaration.Members)
		}
	}
}

func TestRustFocusedExtractionChecksCrossModuleItemVisibility(t *testing.T) {
	root := t.TempDir()
	lib := Source{Path: filepath.Join(root, "src", "lib.rs"), Text: "mod private_model; mod parent; mod outside;\n"}
	privateModel := Source{Path: filepath.Join(root, "src", "private_model.rs"), Text: "struct PrivateModel;\n"}
	parent := Source{Path: filepath.Join(root, "src", "parent.rs"), Text: "pub mod model { pub(super) struct ParentModel; struct PrivateNested; pub mod child { impl super::PrivateNested { pub fn nested(&self) {} } } }\npub mod impls { impl crate::parent::model::ParentModel { pub fn allowed(&self) {} } }\n"}
	outside := Source{Path: filepath.Join(root, "src", "outside.rs"), Text: "impl crate::private_model::PrivateModel { pub fn denied(&self) {} }\nimpl crate::parent::model::ParentModel { pub fn also_denied(&self) {} }\n"}
	analysis, err := Analyze([]Source{lib, privateModel, parent, outside})
	if err != nil {
		t.Fatal(err)
	}
	privateDeclaration := analysis.TSDeclarations[absolutePath(privateModel.Path)+":PrivateModel"]
	parentDeclaration := analysis.TSDeclarations[absolutePath(parent.Path)+":model::ParentModel"]
	nestedDeclaration := analysis.TSDeclarations[absolutePath(parent.Path)+":model::PrivateNested"]
	if privateDeclaration == nil || parentDeclaration == nil || nestedDeclaration == nil {
		t.Fatalf("Rust declarations=%#v", analysis.TSDeclarations)
	}
	assertJVMMember(t, parentDeclaration.Members, Member{Name: "allowed", Kind: "method", Visibility: "public"})
	assertJVMMember(t, nestedDeclaration.Members, Member{Name: "nested", Kind: "method", Visibility: "public"})
	for _, declaration := range []*Declaration{privateDeclaration, parentDeclaration} {
		for _, member := range declaration.Members {
			if member.Name == "denied" || member.Name == "also_denied" {
				t.Fatalf("inaccessible Rust impl attached: %#v", declaration.Members)
			}
		}
	}
}
