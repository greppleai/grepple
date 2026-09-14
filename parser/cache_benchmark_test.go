package parser

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type navigationBenchmarkSource struct {
	path    string
	content string
}

var (
	benchmarkTreeCount int
	benchmarkGraph     NavigationGraph
)

func BenchmarkNavigationParseVersusDiskCache(b *testing.B) {
	sources := navigationBenchmarkSources(b)
	graph := navigationBenchmarkGraph(sources)
	cst := navigationBenchmarkCST(b, sources)
	jsonPath, jsonSize := writeNavigationBenchmarkJSON(b, graph)
	gobPath, gobSize := writeNavigationBenchmarkGob(b, graph)
	protobufMessagePath, protobufMessageSize := writeNavigationBenchmarkProtobufMessages(b, graph)
	protobufPackedPath, protobufPackedSize := writeNavigationBenchmarkProtobufPacked(b, graph)
	cstMessagePath, cstMessageSize := writeNavigationBenchmarkCSTMessages(b, cst)
	cstPackedPath, cstPackedSize := writeNavigationBenchmarkCSTPacked(b, cst)
	totalSourceBytes := navigationBenchmarkSourceBytes(sources)

	b.Run("tree-sitter-parse-memory", func(b *testing.B) {
		benchmarkTreeSitterMemory(b, sources, totalSourceBytes)
	})
	b.Run("tree-sitter-parse-disk", func(b *testing.B) {
		benchmarkTreeSitterDisk(b, sources, totalSourceBytes)
	})
	b.Run("navigation-parse-and-extract", func(b *testing.B) {
		benchmarkNavigationExtraction(b, sources, totalSourceBytes)
	})
	b.Run("full-cst-parse-and-project", func(b *testing.B) {
		benchmarkNavigationCSTProjection(b, sources, totalSourceBytes)
	})

	b.Run("navigation-json-disk", func(b *testing.B) {
		benchmarkNavigationJSONDisk(b, jsonPath, jsonSize)
	})
	b.Run("navigation-gob-disk", func(b *testing.B) {
		benchmarkNavigationGobDisk(b, gobPath, gobSize)
	})
	b.Run("navigation-protobuf-messages-disk", func(b *testing.B) {
		benchmarkNavigationProtobufMessagesDisk(b, protobufMessagePath, protobufMessageSize)
	})
	b.Run("navigation-protobuf-packed-disk", func(b *testing.B) {
		benchmarkNavigationProtobufPackedDisk(b, protobufPackedPath, protobufPackedSize)
	})
	b.Run("full-cst-protobuf-messages-disk", func(b *testing.B) {
		benchmarkNavigationCSTMessagesDisk(b, cstMessagePath, cstMessageSize)
	})
	b.Run("full-cst-protobuf-packed-disk", func(b *testing.B) {
		benchmarkNavigationCSTPackedDisk(b, cstPackedPath, cstPackedSize)
	})
	benchmarkGeneratedProtobufFormats(b, graph, cst)
}

func benchmarkTreeSitterMemory(b *testing.B, sources []navigationBenchmarkSource, totalSourceBytes int) {
	reportNavigationBenchmarkCorpus(b, sources, totalSourceBytes)
	for range b.N {
		count := 0
		for _, source := range sources {
			tree, err := adapterForLanguage("go").Parse(source.content)
			if err != nil {
				b.Fatal(err)
			}
			tree.Close()
			count++
		}
		benchmarkTreeCount = count
	}
}

func benchmarkTreeSitterDisk(b *testing.B, sources []navigationBenchmarkSource, totalSourceBytes int) {
	reportNavigationBenchmarkCorpus(b, sources, totalSourceBytes)
	for range b.N {
		count := 0
		for _, source := range sources {
			content, err := os.ReadFile(source.path)
			if err != nil {
				b.Fatal(err)
			}
			tree, err := adapterForLanguage("go").Parse(string(content))
			if err != nil {
				b.Fatal(err)
			}
			tree.Close()
			count++
		}
		benchmarkTreeCount = count
	}
}

func benchmarkNavigationExtraction(b *testing.B, sources []navigationBenchmarkSource, totalSourceBytes int) {
	reportNavigationBenchmarkCorpus(b, sources, totalSourceBytes)
	for range b.N {
		loaded := NavigationGraph{}
		for _, source := range sources {
			loaded.Merge(BuildNavigationGraph(source.content, "go", source.path))
		}
		benchmarkGraph = loaded
	}
}

func reportNavigationBenchmarkCorpus(b *testing.B, sources []navigationBenchmarkSource, totalSourceBytes int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(totalSourceBytes))
	b.ReportMetric(float64(totalSourceBytes), "source-bytes")
	b.ReportMetric(float64(len(sources)), "source-files")
}

func navigationBenchmarkSources(b *testing.B) []navigationBenchmarkSource {
	b.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		b.Fatal(err)
	}
	var sources []navigationBenchmarkSource
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		sources = append(sources, navigationBenchmarkSource{path: path, content: string(content)})
	}
	if len(sources) == 0 {
		b.Fatal("navigation benchmark corpus is empty")
	}
	return sources
}

func navigationBenchmarkGraph(sources []navigationBenchmarkSource) NavigationGraph {
	graph := NavigationGraph{}
	for _, source := range sources {
		graph.Merge(BuildNavigationGraph(source.content, "go", source.path))
	}
	return graph
}

func navigationBenchmarkSourceBytes(sources []navigationBenchmarkSource) int {
	total := 0
	for _, source := range sources {
		total += len(source.content)
	}
	return total
}

func writeNavigationBenchmarkJSON(b *testing.B, graph NavigationGraph) (string, int) {
	b.Helper()
	content, err := json.Marshal(graph)
	if err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(b.TempDir(), "navigation.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func writeNavigationBenchmarkGob(b *testing.B, graph NavigationGraph) (string, int) {
	b.Helper()
	var content bytes.Buffer
	if err := gob.NewEncoder(&content).Encode(graph); err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(b.TempDir(), "navigation.gob")
	if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
		b.Fatal(err)
	}
	return path, content.Len()
}

func benchmarkNavigationJSONDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		var graph NavigationGraph
		if err := json.Unmarshal(content, &graph); err != nil {
			b.Fatal(err)
		}
		benchmarkGraph = graph
	}
}

func benchmarkNavigationGobDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		var graph NavigationGraph
		if err := gob.NewDecoder(bytes.NewReader(content)).Decode(&graph); err != nil {
			b.Fatal(err)
		}
		benchmarkGraph = graph
	}
}
