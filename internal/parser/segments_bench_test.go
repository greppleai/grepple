package parser

import (
	"fmt"
	"strings"
	"testing"
)

// genLargeGoSource builds a large, match-dense Go file so segment building visits
// many declarations while selecting complete directly matching scopes under a cap.
func genLargeGoSource() (string, map[int]bool) {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "func Func%d(x int) int {\n\ty := x * %d\n\treturn y + %d\n}\n\n", i, i, i)
	}
	content := b.String()
	lines := splitLines(content)
	hits := map[int]bool{}
	for i := 1; i <= len(lines); i += 9 {
		hits[i] = true
	}
	return content, hits
}

func BenchmarkAnalyzeStructureLarge(b *testing.B) {
	content, hits := genLargeGoSource()
	cfg := adapterForLanguage("go")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		analyzeStructure(cfg, content, hits)
	}
}
