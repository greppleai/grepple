package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkNavigationFactCache(b *testing.B) {
	var source strings.Builder
	source.WriteString("package sample\n")
	for index := 0; index < 100; index++ {
		source.WriteString("func Function")
		source.WriteString(string(rune('A' + index%26)))
		source.WriteString("() { Helper() }\n")
	}
	source.WriteString("func Helper() {}\n")
	content := source.String()
	directory := b.TempDir()
	b.Setenv(NavigationCacheDirectoryEnv, directory)
	cachePath := filepath.Join(directory, navigationCacheDigest(content, "go")+".pb")

	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			_ = os.Remove(cachePath)
			b.StartTimer()
			if _, _, hit, err := CachedNavigationGraph(content, "go", "sample/main.go"); err != nil || hit {
				b.Fatalf("cold cache hit=%v err=%v", hit, err)
			}
		}
	})
	b.Run("warm", func(b *testing.B) {
		if _, _, _, err := CachedNavigationGraph(content, "go", "sample/main.go"); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			if _, _, hit, err := CachedNavigationGraph(content, "go", "sample/main.go"); err != nil || !hit {
				b.Fatalf("warm cache hit=%v err=%v", hit, err)
			}
		}
	})
}
