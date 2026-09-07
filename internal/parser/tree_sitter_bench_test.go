package parser

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

const benchGoSource = `package main

import "fmt"

type T struct{ X int }

func (t T) M() int { return t.X }

func main() { fmt.Println(T{X: 1}.M()) }
`

// BenchmarkParseTreePooled measures the current path, which recycles parsers
// through parserPool.
func BenchmarkParseTreePooled(b *testing.B) {
	cfg := configForLanguage("go")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tree, err := parseTree(cfg, benchGoSource)
		if err != nil {
			b.Fatal(err)
		}
		tree.Close()
	}
}

// BenchmarkParseTreeFresh reproduces the old behaviour (a new parser per file)
// so the recycling win is visible in `go test -bench`.
func BenchmarkParseTreeFresh(b *testing.B) {
	cfg := configForLanguage("go")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser := sitter.NewParser()
		if err := parser.SetLanguage(cfg.grammar); err != nil {
			b.Fatal(err)
		}
		tree := parser.Parse([]byte(benchGoSource), nil)
		tree.Close()
		parser.Close()
	}
}
