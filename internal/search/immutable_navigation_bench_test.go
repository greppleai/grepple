package search

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// This benchmark is a warm, immutable synthetic repository, not production latency.
func BenchmarkRelatedImmutableLookupReuse(b *testing.B) {
	root, paths := immutableBenchmarkSources(b)
	for _, revision := range []string{"", "fixed"} {
		label := "rebuild-cached-facts"
		if revision != "" {
			label = "reuse-resolved-lookup"
		}
		b.Run(label, func(b *testing.B) {
			params := Params{Root: root, Query: "Connect", Related: true, FollowRelated: 1, RelatedRepositoryContext: true, ImmutableNavigationRevision: revision, Limit: 20}
			if _, err := Files(params, paths); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for b.Loop() {
				if matches, err := Files(params, paths); err != nil || len(matches) != 20 {
					b.Fatalf("matches=%d err=%v", len(matches), err)
				}
			}
		})
	}
}
func immutableBenchmarkSources(b *testing.B) (string, []string) {
	root := b.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		b.Fatal(err)
	}
	var paths []string
	for i := 0; i < 100; i++ {
		path := filepath.Join(root, fmt.Sprintf("file-%03d.go", i))
		content := fmt.Sprintf("package sample\ntype Item%d struct { Name string }\nfunc Connect%d(value Item%d) Item%d {return value}\n", i, i, i, i)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}
		paths = append(paths, path)
	}
	return root, paths
}
