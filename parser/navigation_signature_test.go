package parser

import (
	"strings"
	"testing"
)

func TestNavigationCallableSignaturesAreSourceHeadersWithoutBodies(t *testing.T) {
	for _, test := range []struct {
		language, source, name, want string
	}{
		{"go", "package sample\nfunc run(\n x struct{ Value int },\n callback func() error,\n) (result interface{ Close() error }, err error) {\n return nil, nil\n}\n", "run", "func run( x struct{ Value int }, callback func() error, ) (result interface{ Close() error }, err error)"},
		{"python", "def run(value: int) -> str:\n    return str(value)\n", "run", "def run(value: int) -> str:"},
		{"typescript", "function run(value: { id: string }): string {\n  return value.id;\n}\n", "run", "function run(value: { id: string }): string"},
		{"typescript", "const run = (value: number): number => value + 1;\n", "run", "(value: number): number =>"},
		{"rust", "pub fn run(value: &str) -> usize { value.len() }\n", "run", "pub fn run(value: &str) -> usize"},
		{"java", "class Demo { public int run(int value) { return value; } }\n", "Demo.run", "public int run(int value)"},
		{"csharp", "class Demo { public int Run(int value) { return value; } }\n", "Demo.Run", "public int Run(int value)"},
		{"c", "int run(int value) { return value; }\n", "run", "int run(int value)"},
		{"cpp", "int run(int value) { return value; }\n", "run", "int run(int value)"},
	} {
		t.Run(test.language, func(t *testing.T) {
			graph := BuildNavigationGraph(test.source, test.language, "source")
			for _, declaration := range graph.Declarations {
				if declaration.Name == test.name {
					if declaration.Signature != test.want || strings.Contains(declaration.Signature, "return ") {
						t.Fatalf("signature=%q want=%q", declaration.Signature, test.want)
					}
					return
				}
			}
			t.Fatalf("missing %s declaration: %+v", test.name, graph.Declarations)
		})
	}
}
