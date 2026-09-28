package parser

import (
	"fmt"
	"strings"
	"testing"
)

var syntaxTraversalSink int

func BenchmarkSyntaxAccessModes(b *testing.B) {
	var source strings.Builder
	source.WriteString("package benchmark\n")
	for index := range 100 {
		fmt.Fprintf(&source, "func Function%d(value int) int { if value > 0 { return value + %d }; return 0 }\n", index, index)
	}
	document, err := NewParser().Parse("go", source.String())
	if err != nil {
		b.Fatal(err)
	}
	defer document.Close()
	snapshot, ok := document.Root().Snapshot()
	if !ok {
		b.Fatal("root snapshot failed")
	}

	b.Run("DocumentNode", func(b *testing.B) { benchmarkDocumentNode(b, document) })
	b.Run("DocumentReadView", func(b *testing.B) { benchmarkDocumentReadView(b, document) })
	b.Run("ImmutableSnapshot", func(b *testing.B) { benchmarkImmutableSnapshot(b, snapshot) })
	b.Run("SnapshotCreation", func(b *testing.B) { benchmarkSnapshotCreation(b, document) })
}

func benchmarkDocumentNode(b *testing.B, document *Document) {
	b.ReportAllocs()
	for range b.N {
		count := 0
		document.WalkNamed(func(node Node) {
			if node.Kind() != "" {
				count++
			}
		})
		syntaxTraversalSink = count
	}
}

func benchmarkDocumentReadView(b *testing.B, document *Document) {
	b.ReportAllocs()
	for range b.N {
		count := 0
		if err := document.Read(func(view DocumentView) error {
			view.Root().WalkNamed(func(node ViewNode) {
				if node.Kind() != "" {
					count++
				}
			})
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		syntaxTraversalSink = count
	}
}

func benchmarkImmutableSnapshot(b *testing.B, snapshot SyntaxNode) {
	b.ReportAllocs()
	for range b.N {
		syntaxTraversalSink = countSnapshotNodes(snapshot)
	}
}

func benchmarkSnapshotCreation(b *testing.B, document *Document) {
	b.ReportAllocs()
	for range b.N {
		created, ok := document.Root().Snapshot()
		if !ok {
			b.Fatal("snapshot failed")
		}
		syntaxTraversalSink = len(created.Children())
	}
}
func countSnapshotNodes(root SyntaxNode) int {
	if !root.Valid() {
		return 0
	}
	count := 0
	stack := []SyntaxNode{root}
	for len(stack) > 0 {
		index := len(stack) - 1
		node := stack[index]
		stack = stack[:index]
		count++
		stack = append(stack, node.Children()...)
	}
	return count
}
