package extract

import (
	"path/filepath"
	"testing"
)

func TestDartFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "lib", "main.dart"), Text: `class Base {}
class Helper {
  String format(String input) => input;
}
class Runner extends Base {
  final Helper helper;
  Runner(this.helper);
  String run(String input) {
    finish();
    return helper.format(input);
  }
  void finish() {}
}
void main() { Runner(Helper()).run('hi'); }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	runner := analysis.ModuleDeclarations[absolutePath(source.Path)+":Runner"]
	if runner == nil || runner.Language != "dart" || !runner.Extends["Base"] {
		t.Fatalf("Dart declaration=%#v", runner)
	}
	assertJVMMember(t, runner.Members, Member{Name: "helper", Kind: "property", Type: "Helper", Visibility: "public"})
	assertJVMMember(t, runner.Members, Member{Name: "run", Kind: "method", Type: "String", Visibility: "public"})
	structure, err := GenerateClassDiagram("Runner", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Dart structure", structure, []string{"class Runner {", "class Base", "class Helper {", "<<dart>> Runner", "Base <|-- Runner", "+helper: Helper"})
	flow, err := GenerateFlowchart("Runner.run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "Dart flow", flow, []string{"%% grepple:language Runner_run dart", "Runner_run --> Runner_finish", "Runner_run --> Helper_format"})
}

func TestDartFocusedTypesIncludeInterfaceAndAccessors(t *testing.T) {
	source := Source{Path: filepath.Join(t.TempDir(), "lib", "user.dart"), Text: `class User implements Comparable<User> {
  String _name;
  User(this._name);
  String get name => _name;
  set name(String value) { _name = value; }
  int compareTo(User other) => name.compareTo(other.name);
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	user := analysis.ModuleDeclarations[absolutePath(source.Path)+":User"]
	if user == nil || !user.Implements["Comparable"] {
		t.Fatalf("Dart implements: %#v", user)
	}
	assertJVMMember(t, user.Members, Member{Name: "_name", Kind: "property", Type: "String", Visibility: "private"})
	assertJVMMember(t, user.Members, Member{Name: "name", Kind: "method", Type: "String", Visibility: "public"})
}
