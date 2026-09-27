package parser

import "testing"

func TestNavigationMemberAccessesAcrossLanguageAdapters(t *testing.T) {
	tests := []struct {
		language  string
		content   string
		member    string
		operation string
	}{
		{"go", "package p\ntype Foo struct{ State int }\nfunc use(value Foo){ _ = value.State }\n", "State", "read"},
		{"javascript", "class Foo { run() {} }\nfunction use(){ return Foo.state }\n", "state", "read"},
		{"typescript", "class Foo { state: number = 0 }\nfunction use(value: Foo){ return value.state }\n", "state", "read"},
		{"tsx", "class Foo { state: number = 0 }\nfunction use(value: Foo){ return value.state }\n", "state", "read"},
		{"python", "class Foo:\n    state = 0\ndef use():\n    return Foo.state\n", "state", "read"},
		{"java", "class Foo { int state; }\nclass Use { int run(Foo value){ return value.state; } }\n", "state", "read"},
		{"kotlin", "class Foo(var state: Int)\nfun use(value: Foo): Int { return value.state }\n", "state", "read"},
		{"csharp", "class Foo { public int State; }\nclass Use { int Run(Foo value){ return value.State; } }\n", "State", "read"},
		{"c", "typedef struct { int state; } Foo;\nint use(Foo value){ return value.state; }\n", "state", "read"},
		{"cpp", "struct Foo { int state; };\nint use(Foo value){ return value.state; }\n", "state", "read"},
		{"rust", "struct Foo { state: i32 }\nfn use(value: Foo) -> i32 { value.state }\n", "state", "read"},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, "sample")
			for _, access := range graph.MemberAccesses {
				if access.Member == test.member && access.Operation == test.operation {
					return
				}
			}
			t.Fatalf("member %s(%s) not found: %#v", test.member, test.operation, graph.MemberAccesses)
		})
	}
}

func TestNavigationMemberAccessClassifiesWrites(t *testing.T) {
	content := "package p\ntype Foo struct{ State int }\nfunc use(value Foo){ value.State = 1 }\n"
	graph := BuildNavigationGraph(content, "go", "sample.go")
	if len(graph.MemberAccesses) != 1 || graph.MemberAccesses[0].Operation != "write" || graph.MemberAccesses[0].ReceiverType != "Foo" {
		t.Fatalf("accesses=%#v", graph.MemberAccesses)
	}
	if len(graph.TypeUsages) != 1 || graph.TypeUsages[0].Type != "Foo" {
		t.Fatalf("type usages=%#v", graph.TypeUsages)
	}
}

func TestNavigationTypeUsagesPreserveImportAndRole(t *testing.T) {
	content := `package p
import sitter "github.com/tree-sitter/go-tree-sitter"
func Public(node *sitter.Node) *sitter.Node {
var local *sitter.Node = node
return local
}`
	graph := BuildNavigationGraph(content, "go", "sample.go")
	if len(graph.Imports) != 1 || graph.Imports[0].Alias != "sitter" || graph.Imports[0].ImportPath != "github.com/tree-sitter/go-tree-sitter" || graph.Imports[0].Line != 2 {
		t.Fatalf("imports=%#v", graph.Imports)
	}
	if len(graph.TypeUsages) != 3 {
		t.Fatalf("type usages=%#v", graph.TypeUsages)
	}
	roles := make(map[string]NavigationTypeUsage)
	for _, usage := range graph.TypeUsages {
		roles[usage.Role] = usage
	}
	for _, role := range []string{"local", "parameter", "result"} {
		usage, ok := roles[role]
		if !ok || usage.Type != "Node" || usage.ImportPath != "github.com/tree-sitter/go-tree-sitter" {
			t.Fatalf("role %q usage=%#v all=%#v", role, usage, graph.TypeUsages)
		}
	}
}

func TestNavigationImportsRetainBlankAndDotEvidence(t *testing.T) {
	content := "package p\nimport _ \"example.com/sideeffect\"\nimport . \"example.com/dot\"\n"
	graph := BuildNavigationGraph(content, "go", "sample.go")
	if len(graph.Imports) != 2 || graph.Imports[0].Alias != "." || graph.Imports[1].Alias != "_" {
		t.Fatalf("imports=%#v", graph.Imports)
	}
}
