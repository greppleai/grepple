package architecture

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func BenchmarkDirectoryArchitecture(b *testing.B) {
	root := b.TempDir()
	writeArchitectureFixture(b, root, "go.mod", "module example.com/architecturebench\n")
	for index := range 20 {
		writeArchitectureFixture(b, root, fmt.Sprintf("service%d/service.go", index), fmt.Sprintf("package service%d\ntype Service%d struct{}\nfunc Run%d() {}\n", index, index, index))
	}
	b.ReportAllocs()
	var outputBytes int
	for range b.N {
		architecture, err := buildDirectoryArchitecture([]string{root}, 0)
		if err != nil {
			b.Fatal(err)
		}
		content, err := json.Marshal(architecture)
		if err != nil {
			b.Fatal(err)
		}
		outputBytes += len(content)
	}
	if b.N > 0 {
		b.ReportMetric(float64(outputBytes)/float64(b.N), "output_bytes/op")
	}
}

// BenchmarkDirectoryArchitectureCheckout profiles a real checkout, opt-in only.
// Set GREPPLE_BENCH_REPO to an explicit checkout root; this fixture is never used by CI.
func BenchmarkDirectoryArchitectureCheckout(b *testing.B) {
	root := os.Getenv("GREPPLE_BENCH_REPO")
	if root == "" {
		b.Skip("set GREPPLE_BENCH_REPO to an existing repository checkout")
	}
	chdirForConfigTest(b, root)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := buildDirectoryArchitecture([]string{"."}, 0); err != nil {
			b.Fatal(err)
		}
	}
}
