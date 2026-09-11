//go:build native_tree_cache

// This benchmark requires a go-tree-sitter build with the experimental native
// tree serialization API. Supply that fork with a go.work or temporary module replace.

package parser

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

var benchmarkNativeTreeBytes []byte

func BenchmarkNativeTreeCache(b *testing.B) {
	sources := navigationBenchmarkSources(b)
	sourceBytes := navigationBenchmarkSourceBytes(sources)
	path, cacheSize := writeNativeTreeBenchmarkCache(b, sources)
	b.Run("serialize-memory", func(b *testing.B) {
		benchmarkNativeTreeSerializeMemory(b, sources, sourceBytes)
	})
	b.Run("deserialize-memory", func(b *testing.B) {
		benchmarkNativeTreeMemory(b, path, cacheSize, len(sources))
	})
	b.Run("disk", func(b *testing.B) {
		benchmarkNativeTreeDisk(b, path, cacheSize, len(sources))
	})
}

func benchmarkNativeTreeSerializeMemory(b *testing.B, sources []navigationBenchmarkSource, sourceBytes int) {
	trees := make([]*sitter.Tree, 0, len(sources))
	for _, source := range sources {
		tree, err := parseTree(adapterForLanguage("go"), source.content)
		if err != nil {
			b.Fatal(err)
		}
		trees = append(trees, tree)
	}
	defer func() {
		for _, tree := range trees {
			tree.Close()
		}
	}()
	b.ReportAllocs()
	b.SetBytes(int64(sourceBytes))
	b.ReportMetric(float64(len(sources)), "source-files")
	b.ResetTimer()
	for range b.N {
		var last []byte
		for _, tree := range trees {
			serialized, err := tree.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
			last = serialized
		}
		benchmarkNativeTreeBytes = last
	}
}

func benchmarkNativeTreeMemory(b *testing.B, path string, cacheSize, sourceFiles int) {
	content, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	grammar := adapterForLanguage("go").Grammar()
	b.ReportAllocs()
	b.SetBytes(int64(cacheSize))
	b.ReportMetric(float64(cacheSize), "cache-bytes")
	b.ReportMetric(float64(sourceFiles), "source-files")
	b.ResetTimer()
	for range b.N {
		count, err := deserializeNativeTrees(content, grammar)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkTreeCount = count
	}
}

func writeNativeTreeBenchmarkCache(b *testing.B, sources []navigationBenchmarkSource) (string, int) {
	b.Helper()
	content := make([]byte, 0)
	for _, source := range sources {
		tree, err := parseTree(adapterForLanguage("go"), source.content)
		if err != nil {
			b.Fatal(err)
		}
		serialized, err := tree.MarshalBinary()
		tree.Close()
		if err != nil {
			b.Fatal(err)
		}
		content = binary.LittleEndian.AppendUint32(content, uint32(len(serialized)))
		content = append(content, serialized...)
	}
	path := filepath.Join(b.TempDir(), "native-trees.cache")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func benchmarkNativeTreeDisk(b *testing.B, path string, cacheSize, sourceFiles int) {
	b.Helper()
	grammar := adapterForLanguage("go").Grammar()
	b.ReportAllocs()
	b.SetBytes(int64(cacheSize))
	b.ReportMetric(float64(cacheSize), "cache-bytes")
	b.ReportMetric(float64(sourceFiles), "source-files")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		count, err := deserializeNativeTrees(content, grammar)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkTreeCount = count
	}
}

func deserializeNativeTrees(content []byte, grammar *sitter.Language) (int, error) {
	count := 0
	for len(content) > 0 {
		if len(content) < 4 {
			return 0, fmt.Errorf("truncated native tree length")
		}
		length := int(binary.LittleEndian.Uint32(content))
		content = content[4:]
		if length > len(content) {
			return 0, fmt.Errorf("truncated native tree payload")
		}
		tree, err := sitter.DeserializeTree(content[:length], grammar)
		if err != nil {
			return 0, err
		}
		tree.Close()
		content = content[length:]
		count++
	}
	return count, nil
}
