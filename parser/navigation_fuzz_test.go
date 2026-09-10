package parser

import "testing"

func FuzzBuildNavigationGraph(f *testing.F) {
	seeds := []struct {
		language, content string
	}{
		{"go", "package sample\r\nfunc Start() { Finish() }\r\nfunc Finish() {}\r\n"},
		{"typescript", "class Store { load<T>(value: T): T { return value }"},
		{"python", "@decorator\nasync def run(\n    helper()"},
		{"java", "class Service { <T> T run(T value) { return helper(value); }"},
		{"rust", "fn run<T>(value: T) { helper(value);"},
		{"shell", "run() { helper"},
	}
	for _, seed := range seeds {
		f.Add(seed.language, seed.content)
	}
	f.Fuzz(func(t *testing.T, language, content string) {
		if len(content) > 64*1024 {
			t.Skip()
		}
		graph := BuildNavigationGraph(content, language, "fuzz-input")
		for _, declaration := range graph.Declarations {
			if declaration.Start < 1 || declaration.End < declaration.Start {
				t.Fatalf("invalid declaration range: %+v", declaration)
			}
		}
		for _, call := range graph.Calls {
			if call.Line < 1 || call.EnclosingStart < 1 || call.EnclosingEnd < call.EnclosingStart {
				t.Fatalf("invalid call range: %+v", call)
			}
		}
	})
}
