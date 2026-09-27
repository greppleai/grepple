package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkPrepareSourcePathsSharedDirectories(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/bench\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	sources := make([]Source, 0, 1200)
	for directory := 0; directory < 120; directory++ {
		name := filepath.Join(root, fmt.Sprintf("pkg%03d", directory))
		for file := 0; file < 10; file++ {
			sources = append(sources, Source{Path: filepath.Join(name, fmt.Sprintf("file%02d.go", file))})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prepareSourcePaths(newAnalysis(), sources)
	}
}
