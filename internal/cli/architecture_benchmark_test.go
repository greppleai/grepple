package cli

import (
	"encoding/json"
	"fmt"
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
