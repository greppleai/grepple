package parser

import "testing"

func TestDartSyntaxOutlineAndNavigation(t *testing.T) {
	const content = "import 'src/helper.dart' as helpers;\nclass Service {\n final String title;\n Service(this.title);\n String greet(String text) { return helpers.format(text) + title; }\n}\nvoid _hidden() {}\nvoid main() { final service = Service('hi'); service.greet('world'); }\n"
	document, err := NewParser().Parse("dart", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	outline := NewParser().Outline(document, "lib/main.dart")
	mustFind(t, outline.Symbols, "method", "greet")
	mustFind(t, outline.Symbols, "class", "Service")
	graph := NewParser().NavigationGraph(document, "lib/main.dart")
	if len(graph.Imports) != 1 || graph.Imports[0].ImportPath != "src/helper.dart" || graph.Imports[0].Alias != "helpers" {
		t.Fatalf("Dart imports: %+v", graph.Imports)
	}
	declarations := map[string]NavigationDeclaration{}
	for _, declaration := range graph.Declarations {
		declarations[declaration.Name] = declaration
	}
	if declarations["Service.greet"].Kind != "method" || declarations["Service.greet"].Start != 5 || declarations["main"].Entrypoint != "process" || declarations["_hidden"].Visibility != NavigationVisibilityNonPublic {
		t.Fatalf("Dart declarations: %+v", graph.Declarations)
	}
	if len(graph.TypeDeclarations) != 1 || graph.TypeDeclarations[0].Name != "Service" {
		t.Fatalf("Dart type declarations: %+v", graph.TypeDeclarations)
	}
	if len(graph.Fields) != 1 || graph.Fields[0].OwnerType != "Service" || graph.Fields[0].Name != "title" || graph.Fields[0].Type != "String" {
		t.Fatalf("Dart fields: %+v", graph.Fields)
	}
	seen := map[string]bool{}
	for _, call := range graph.Calls {
		seen[call.Display] = true
	}
	for _, display := range []string{"helpers.format", "Service", "service.greet"} {
		if !seen[display] {
			t.Fatalf("missing Dart call %q in %+v", display, graph.Calls)
		}
	}
}

func TestDartMainRequiresRunnableTopLevelFunction(t *testing.T) {
	for _, content := range []string{
		"class App { void main() {} }",
		"part of 'app.dart'; void main() {}",
		"int main() => 1;",
		"void main(List<String> args) {}",
	} {
		graph := BuildNavigationGraph(content, "dart", "lib/sample.dart")
		for _, declaration := range graph.Declarations {
			if declaration.Entrypoint != "" {
				t.Fatalf("false entrypoint in %q: %+v", content, declaration)
			}
		}
	}
}

func TestDartConditionalImportsDoNotChooseOnePlatform(t *testing.T) {
	content := "import 'src/default.dart' if (dart.library.html) 'src/web.dart';\nvoid main() { fallback(); }\n"
	graph := BuildNavigationGraph(content, "dart", "lib/main.dart")
	if len(graph.Imports) != 0 {
		t.Fatalf("conditional import has no provable target: %+v", graph.Imports)
	}
}

func TestDartModernDeclarationShapes(t *testing.T) {
	const content = `typedef Task = Future<int> Function(String input);
enum State { ready, done }
mixin Tagged { String get tag => 'ok'; }
extension type Id(int value) { int get raw => value; }
class User {
  String _name;
  User(this._name);
  String get name => _name;
  set name(String value) { _name = value; }
}
`
	outline := OutlineFile("lib/model.dart", content)
	for _, item := range []struct{ kind, name string }{{"type", "Task"}, {"enum", "State"}, {"const", "ready"}, {"mixin", "Tagged"}, {"extension", "Id"}, {"method", "name"}} {
		mustFind(t, outline.Symbols, item.kind, item.name)
	}
	graph := BuildNavigationGraph(content, "dart", "lib/model.dart")
	foundGetter, foundSetter := false, false
	for _, declaration := range graph.Declarations {
		if declaration.Name == "User.name" && declaration.Kind == "method" {
			if declaration.ResultType == "String" {
				foundGetter = true
			} else {
				foundSetter = true
			}
		}
	}
	if !foundGetter || !foundSetter {
		t.Fatalf("Dart getter/setter navigation: %+v", graph.Declarations)
	}
}
