package directorymeta

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkAreaIndexManyDirectories(b *testing.B) {
	root := b.TempDir()
	paths := make([]string, 0, 1200)
	for directory := 0; directory < 120; directory++ {
		name := fmt.Sprintf("pkg%03d", directory)
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			b.Fatal(err)
		}
		for file := 0; file < 10; file++ {
			paths = append(paths, fmt.Sprintf("%s/file%02d.go", name, file))
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := AreaIndex(root, paths); err != nil {
			b.Fatal(err)
		}
	}
}
