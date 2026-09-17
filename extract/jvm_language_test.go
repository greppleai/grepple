package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "Runner.java"), Text: `interface Task {
    boolean execute(String value);
}
class Base {
    protected void close() {}
}
class Helper {
    boolean run(String value) { return true; }
}
class Runner extends Base implements Task {
    private final Helper helper;
    Runner(Helper helper) { this.helper = helper; }
    public boolean execute(String value) { finish(); return helper.run(value); }
    private void finish() {}
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	runner := analysis.ModuleDeclarations[absolutePath(source.Path)+":Runner"]
	if runner == nil || runner.Language != "java" || !runner.Extends["Base"] || !runner.Implements["Task"] {
		t.Fatalf("Java declaration=%#v", runner)
	}
	assertJVMMember(t, runner.Members, Member{Name: "helper", Kind: "property", Type: "Helper", Visibility: "private"})
	assertJVMMember(t, runner.Members, Member{Name: "execute", Kind: "method", Type: "boolean", Visibility: "public"})
	task := analysis.ModuleDeclarations[absolutePath(source.Path)+":Task"]
	assertJVMMember(t, task.Members, Member{Name: "execute", Kind: "method", Type: "boolean", Visibility: "public"})

	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Java structure", structure, []string{"class Runner {", "class Base {", "class Task {", "class Helper {", "<<java>> Runner", "Base <|-- Runner", "Task <|.. Runner", "-helper: Helper"})
	flow, err := GenerateFlowchart("Runner.execute", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Java flow", flow, []string{"%% grepple:language Runner_execute java", "Runner_execute --> Runner_finish", "Runner_execute --> Helper_run"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestKotlinFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "Runner.kt"), Text: `interface Task {
    fun execute(value: String): Boolean
}
open class Base {
    fun close() {}
}
class Helper {
    fun run(value: String): Boolean = true
}
class Runner(private val helper: Helper, val name: String) : Base(), Task {
    override fun execute(value: String): Boolean {
        finish()
        return helper.run(value)
    }
    private fun finish() {}
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	runner := analysis.ModuleDeclarations[absolutePath(source.Path)+":Runner"]
	if runner == nil || runner.Language != "kotlin" || !runner.Extends["Base"] || !runner.Implements["Task"] {
		t.Fatalf("Kotlin declaration=%#v", runner)
	}
	assertJVMMember(t, runner.Members, Member{Name: "helper", Kind: "property", Type: "Helper", Visibility: "private"})
	assertJVMMember(t, runner.Members, Member{Name: "name", Kind: "property", Type: "string", Visibility: "public"})
	assertJVMMember(t, runner.Members, Member{Name: "execute", Kind: "method", Type: "boolean", Visibility: "public"})

	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Kotlin structure", structure, []string{"class Runner {", "class Base {", "class Task {", "class Helper {", "<<kotlin>> Runner", "Base <|-- Runner", "Task <|.. Runner", "-helper: Helper"})
	flow, err := GenerateFlowchart("Runner.execute", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Kotlin flow", flow, []string{"%% grepple:language Runner_execute kotlin", "Runner_execute --> Runner_finish", "Runner_execute --> Helper_run"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestJVMFocusedExtractionResolvesUniqueCrossFileDependencies(t *testing.T) {
	tests := []jvmCrossFileCase{
		{"java", ".java", "class Entry extends Base { void start() { finish(); } }", "class Base {}", "class Helper { void finish() {} }", "Base <|-- Entry"},
		{"kotlin", ".kt", "class Entry : Base() {\n    fun start() { finish() }\n}\n", "open class Base\n", "class Helper {\n    fun finish() {}\n}\n", "Base <|-- Entry"},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) { assertJVMCrossFileDependencies(t, test) })
	}
}

type jvmCrossFileCase struct {
	language, extension, entryText, baseText, helperText, relation string
}

func assertJVMCrossFileDependencies(t *testing.T, test jvmCrossFileCase) {
	t.Helper()
	root := t.TempDir()
	entry := Source{Path: filepath.Join(root, "Entry"+test.extension), Text: test.entryText}
	base := Source{Path: filepath.Join(root, "Base"+test.extension), Text: test.baseText}
	helper := Source{Path: filepath.Join(root, "Helper"+test.extension), Text: test.helperText}
	sources := []Source{entry, base, helper}
	structure, err := GenerateClassDiagram("Entry", entry, sources, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(structure, test.relation) {
		t.Fatalf("cross-file relation missing:\n%s", structure)
	}
	flow, err := GenerateFlowchart("Entry.start", entry.Path, sources, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flow, "Entry_start --> Helper_finish") {
		t.Fatalf("cross-file call missing:\n%s", flow)
	}
	analysis, err := Analyze(sources)
	if err != nil {
		t.Fatal(err)
	}
	if confidenceForCall(analysis, "finish") != "unique-terminal" {
		t.Fatalf("cross-file confidence=%q", confidenceForCall(analysis, "finish"))
	}
}

func TestJVMFocusedExtractionSupportsModernTypeShapes(t *testing.T) {
	root := t.TempDir()
	java := Source{Path: filepath.Join(root, "Types.java"), Text: "record User(String name, int age) {}\nenum State { READY, DONE }\n"}
	kotlin := Source{Path: filepath.Join(root, "Types.kt"), Text: "data class Profile(val name: String, val age: Int)\nobject Registry {\n    val current: Profile? = null\n}\n"}
	analysis, err := Analyze([]Source{java, kotlin})
	if err != nil {
		t.Fatal(err)
	}
	user := analysis.ModuleDeclarations[absolutePath(java.Path)+":User"]
	assertJVMMember(t, user.Members, Member{Name: "name", Kind: "property", Type: "string", Visibility: "package"})
	assertJVMMember(t, user.Members, Member{Name: "age", Kind: "property", Type: "number", Visibility: "package"})
	state := analysis.ModuleDeclarations[absolutePath(java.Path)+":State"]
	if state == nil || len(state.Members) != 2 || !state.Members[0].Static {
		t.Fatalf("Java enum=%#v", state)
	}
	profile := analysis.ModuleDeclarations[absolutePath(kotlin.Path)+":Profile"]
	assertJVMMember(t, profile.Members, Member{Name: "name", Kind: "property", Type: "string", Visibility: "public"})
	assertJVMMember(t, profile.Members, Member{Name: "age", Kind: "property", Type: "number", Visibility: "public"})
	registry := analysis.ModuleDeclarations[absolutePath(kotlin.Path)+":Registry"]
	if registry == nil || len(registry.Members) != 1 || !registry.Members[0].Static || registry.Members[0].Type != "Profile?" {
		t.Fatalf("Kotlin object=%#v", registry)
	}
}

func TestJVMFocusedFlowsDoNotGuessAmbiguousCrossFileTargets(t *testing.T) {
	for _, test := range []struct {
		language, extension, entryText, helperText string
	}{
		{"java", ".java", "class Entry { void start() { finish(); } }", "class Helper { void finish() {} }"},
		{"kotlin", ".kt", "class Entry {\n    fun start() {\n        finish()\n    }\n}\n", "class Helper {\n    fun finish() {}\n}\n"},
	} {
		t.Run(test.language, func(t *testing.T) {
			root := t.TempDir()
			entry := Source{Path: filepath.Join(root, "Entry"+test.extension), Text: test.entryText}
			first := Source{Path: filepath.Join(root, "First"+test.extension), Text: test.helperText}
			secondText := strings.Replace(test.helperText, "Helper", "Other", 1)
			second := Source{Path: filepath.Join(root, "Second"+test.extension), Text: secondText}
			flow, err := GenerateFlowchart("Entry.start", entry.Path, []Source{entry, first, second}, GenerateOptions{Depth: 2, MaxNodes: 20})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(flow, "Entry_start -->") {
				t.Fatalf("ambiguous %s call was guessed:\n%s", test.language, flow)
			}
		})
	}
}

func confidenceForCall(analysis *Analysis, name string) string {
	for _, call := range analysis.Navigation.Calls {
		if call.Name == name {
			return call.Confidence
		}
	}
	return ""
}

func assertJVMMember(t *testing.T, members []Member, expected Member) {
	t.Helper()
	for _, member := range members {
		if member.Name == expected.Name && member.Kind == expected.Kind && member.Type == expected.Type && member.Visibility == expected.Visibility {
			return
		}
	}
	t.Fatalf("member %#v missing from %#v", expected, members)
}

func assertArchitectureRoundTrip(t *testing.T, structure, flow string, sources []Source) {
	t.Helper()
	if diagnostics, err := CheckClassDiagram(structure, sources); err != nil || len(diagnostics) != 0 {
		t.Fatalf("structure diagnostics=%v err=%v", diagnostics, err)
	}
	if diagnostics, err := CheckFlowchart(flow, sources); err != nil || len(diagnostics) != 0 {
		t.Fatalf("flow diagnostics=%v err=%v", diagnostics, err)
	}
}
