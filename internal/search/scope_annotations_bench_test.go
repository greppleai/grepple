package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run separately with GREPPLE_WORKERS=1 and =8: WorkerCount is process-wide.
// Legacy line ranges stay enabled in both modes, matching real --line-only.
func BenchmarkScopeAnnotationsWindowedFiles(b *testing.B) {
	for _, hits := range []int{1, 100, 1000} {
		for _, extraLines := range []int{0, 2000} {
			for _, files := range []int{1, 8} {
				dir := b.TempDir()
				paths := make([]string, files)
				var content strings.Builder
				content.WriteString("package p\nfunc run() {\n")
				for i := range hits {
					fmt.Fprintf(&content, " work(%d)\n", i)
				}
				for i := range extraLines {
					fmt.Fprintf(&content, " // padding %d\n", i)
				}
				content.WriteString("}\n")
				for i := range files {
					paths[i] = filepath.Join(dir, fmt.Sprintf("%02d.go", i))
					if err := os.WriteFile(paths[i], []byte(content.String()), 0600); err != nil {
						b.Fatal(err)
					}
				}
				for _, window := range []int{1, files} {
					for _, enabled := range []bool{false, true} {
						name := fmt.Sprintf("hits=%d/pad=%d/files=%d/window=%d/scopes=%t", hits, extraLines, files, window, enabled)
						b.Run(name, func(b *testing.B) {
							params := Params{Query: "work", Regex: false, Root: dir, Limit: window, SkipSegments: true, LineRanges: true, AnnotateScopes: enabled}
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								results, err := Files(params, paths)
								if err != nil {
									b.Fatal(err)
								}
								if len(results) != window {
									b.Fatalf("returned %d files, want %d", len(results), window)
								}
							}
						})
					}
				}
			}
		}
	}
}
