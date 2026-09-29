package parser

import "testing"

func TestSwiftSyntaxOutlineAndNavigationFacts(t *testing.T) {
	const content = `import Foundation
public protocol Named { func greet(_ name: String) -> String }
struct User: Named {
    let title: String
    var inferred = 1
    init(title: String) { self.title = title }
    func greet(_ name: String) -> String { return title + name }
}
enum State { case ready, done }
func run(value: User) -> String {
    let user = User(title: "hello")
    return user.greet("world")
}
`
	document, err := NewParser().Parse("swift", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	outline := NewParser().Outline(document, "Sources/App.swift")
	for _, wanted := range []struct{ kind, name string }{{"interface", "Named"}, {"struct", "User"}, {"constructor", "init"}, {"method", "greet"}, {"enum", "State"}, {"const", "ready"}, {"function", "run"}} {
		mustFind(t, outline.Symbols, wanted.kind, wanted.name)
	}
	graph := NewParser().NavigationGraph(document, "Sources/App.swift")
	if len(graph.Imports) != 1 || graph.Imports[0].ImportPath != "Foundation" {
		t.Fatalf("Swift imports: %+v", graph.Imports)
	}
	declarations := make(map[string]NavigationDeclaration)
	for _, declaration := range graph.Declarations {
		declarations[declaration.Name] = declaration
	}
	if declarations["User.greet"].Kind != "method" || declarations["User.init"].Kind != "constructor" || declarations["run"].Kind != "function" || declarations["run"].Entrypoint != "" {
		t.Fatalf("Swift callables: %+v", graph.Declarations)
	}
	if declarations["Named.greet"].Visibility != NavigationVisibilityPublic {
		t.Fatalf("explicit public protocol visibility: %+v", graph.Declarations)
	}
	assertSwiftTypedFields(t, graph)
	assertSwiftCalls(t, graph)
}

func assertSwiftTypedFields(t *testing.T, graph NavigationGraph) {
	t.Helper()
	var typed, inferred bool
	for _, field := range graph.Fields {
		if field.OwnerType == "User" && field.Name == "title" && field.Type == "String" {
			typed = true
		}
		if field.Name == "inferred" {
			inferred = true
		}
	}
	if !typed || inferred {
		t.Fatalf("Swift typed fields: %+v", graph.Fields)
	}
}

func assertSwiftCalls(t *testing.T, graph NavigationGraph) {
	t.Helper()
	seen := map[string]bool{}
	for _, call := range graph.Calls {
		seen[call.Display] = true
	}
	for _, display := range []string{"User", "user.greet"} {
		if !seen[display] {
			t.Fatalf("missing Swift call %q in %+v", display, graph.Calls)
		}
	}
}

func TestSwiftVisibilityAndNoGuessedEntrypoint(t *testing.T) {
	graph := BuildNavigationGraph("private func hidden() {}\npublic func shown() {}\nfunc main() {}\n", "swift", "Sources/Main.swift")
	declarations := map[string]NavigationDeclaration{}
	for _, declaration := range graph.Declarations {
		declarations[declaration.Name] = declaration
	}
	if declarations["hidden"].Visibility != NavigationVisibilityNonPublic || declarations["shown"].Visibility != NavigationVisibilityPublic || declarations["main"].Entrypoint != "" {
		t.Fatalf("Swift visibility and entrypoint: %+v", graph.Declarations)
	}
}
