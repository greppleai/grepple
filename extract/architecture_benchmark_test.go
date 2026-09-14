package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkArchitectureWorkflows(b *testing.B) {
	_, sources := writeArchitectureBenchmarkFixture(b)
	entry := sources[0]
	b.Run("FocusedStructure", func(b *testing.B) {
		benchmarkFocusedStructure(b, entry, sources)
	})
	b.Run("FocusedFlow", func(b *testing.B) {
		benchmarkFocusedFlow(b, entry, sources)
	})
}

func benchmarkFocusedStructure(b *testing.B, entry Source, sources []Source) {
	b.ReportAllocs()
	var bytes int
	for range b.N {
		output, err := GenerateClassDiagram("Service", entry, sources, GenerateOptions{Depth: 2, MaxNodes: 30})
		if err != nil {
			b.Fatal(err)
		}
		bytes += len(output)
	}
	reportArchitectureBytes(b, bytes)
}

func benchmarkFocusedFlow(b *testing.B, entry Source, sources []Source) {
	b.ReportAllocs()
	var bytes int
	for range b.N {
		output, err := GenerateFlowchart("Service.Run", entry.Path, sources, GenerateOptions{Depth: 2, DepthSet: true, MaxNodes: 30})
		if err != nil {
			b.Fatal(err)
		}
		bytes += len(output)
	}
	reportArchitectureBytes(b, bytes)
}

func reportArchitectureBytes(b *testing.B, total int) {
	if b.N > 0 {
		b.ReportMetric(float64(total)/float64(b.N), "output_bytes/op")
	}
}

func writeArchitectureBenchmarkFixture(tb testing.TB) (string, []Source) {
	tb.Helper()
	root := tb.TempDir()
	writeArchitectureBenchmarkFile(tb, filepath.Join(root, "go.mod"), "module example.com/architecturebench\n")
	for index := range 8 {
		content := fmt.Sprintf("package model\ntype Model%d struct { ID string; Count int }\nfunc NewModel%d() *Model%d { return &Model%d{} }\n", index, index, index, index)
		writeArchitectureBenchmarkFile(tb, filepath.Join(root, "model", fmt.Sprintf("model%d.go", index)), content)
	}
	writeArchitectureBenchmarkFile(tb, filepath.Join(root, "service", "service.go"), "package service\nimport \"example.com/architecturebench/model\"\nfunc Run() *model.Model0 { return model.NewModel0() }\n")

	sources := []Source{
		{Path: filepath.Join(root, "web", "service.ts"), Text: "import { Repository } from './repository'; export class Service { constructor(private repo: Repository) {} Run(): void { this.Validate(); this.repo.Save(); } Validate(): void {} }"},
		{Path: filepath.Join(root, "web", "repository.ts"), Text: "export class Repository { Save(): void { flush(); } } export function flush(): void {}"},
		{Path: filepath.Join(root, "web", "model.ts"), Text: "export interface Record { id: string }"},
	}
	return root, sources
}

func writeArchitectureBenchmarkFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}
