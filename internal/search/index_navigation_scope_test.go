package search

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestIndexedRelatedPageUsesCompleteMatchedRepositoryOnly(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "owner", "selected")
	other := filepath.Join(root, "owner", "other")
	for _, dir := range []string{selected, other} {
		mustCreateNavigationDirectory(t, filepath.Join(dir, ".git"))
	}
	mustWriteNavigationFile(t, filepath.Join(selected, "go.mod"), "module example.com/selected\n")
	mustWriteNavigationFile(t, filepath.Join(selected, "types.go"), "package selected\ntype Request struct { Name string }\n")
	mustWriteNavigationFile(t, filepath.Join(selected, "helper.go"), "package selected\nfunc helper(request Request) Request {return request}\n")
	var candidates []string
	for i := 0; i < 40; i++ {
		p := filepath.Join(selected, fmt.Sprintf("match-%02d.go", i))
		mustWriteNavigationFile(t, p, fmt.Sprintf("package selected\nfunc Call%d(request Request) Request { return helper(request) /* PAGE_NEEDLE */ }\n", i))
		candidates = append(candidates, p)
	}
	// Unreturned repositories must not become navigation roots.
	mustWriteNavigationFile(t, filepath.Join(other, "broken.go"), "this is deliberately not valid Go")
	before := readInvocations.Load()
	matches, err := Files(Params{Root: root, Query: "PAGE_NEEDLE", Globs: []string{"**/match-*.go"}, Limit: 20, Related: true, RelatedRepositoryContext: true}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	// A bounded parallel scan can finish its current worker batch (not the tail).
	batch := max(1, WorkerCount()*8)
	maxReads := min(len(candidates), ((20+batch-1)/batch)*batch)
	if len(matches) != 20 || readInvocations.Load()-before > int64(maxReads) {
		t.Fatalf("page=%d reads=%d", len(matches), readInvocations.Load()-before)
	}
	roots, err := relatedNavigationRoots(root, matches)
	if err != nil || len(roots) != 1 || roots[0] != selected {
		t.Fatalf("navigation roots=%v error=%v", roots, err)
	}
	foundHelper := false
	for _, p := range matches[0].Related {
		if p.Name == "helper" {
			foundHelper = true
		}
	}
	if !foundHelper || findRelatedTypePoint(matches[0].Related, "Request") == nil {
		t.Fatalf("index omitted complete nonmatching declaration context: %+v", matches[0].Related)
	}
}

func TestExplicitNavigationRootNeverEscapesForAncestorMarkers(t *testing.T) {
	parent := t.TempDir()
	mustCreateNavigationDirectory(t, filepath.Join(parent, ".git"))
	root := filepath.Join(parent, "confined")
	mustCreateNavigationDirectory(t, root)
	inside := filepath.Join(root, "source.go")
	mustWriteNavigationFile(t, inside, "package confined\n")
	roots, err := relatedNavigationRoots(root, []FileMatch{{File: inside}, {File: filepath.Join(parent, "outside.go")}})
	if err != nil || len(roots) != 1 || roots[0] != root {
		t.Fatalf("roots=%v err=%v", roots, err)
	}
}

func BenchmarkIndexedRelatedMatchedRepositoryPage(b *testing.B) {
	root := b.TempDir()
	var candidates []string
	for r := 0; r < 100; r++ {
		dir := filepath.Join(root, "owner", fmt.Sprintf("repo-%03d", r))
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0700); err != nil {
			b.Fatal(err)
		}
		for f := 0; f < 20; f++ {
			path := filepath.Join(dir, fmt.Sprintf("file-%02d.go", f))
			if err := os.WriteFile(path, []byte(fmt.Sprintf("package sample\nfunc F%d() { /* BENCH_NEEDLE */ }\n", f)), 0600); err != nil {
				b.Fatal(err)
			}
			candidates = append(candidates, path)
		}
	}
	for _, completeMatchedRepository := range []bool{false, true} {
		label := "whole-corpus-navigation"
		if completeMatchedRepository {
			label = "matched-repository-navigation"
		}
		b.Run(label, func(b *testing.B) {
			params := Params{Root: root, Query: "BENCH_NEEDLE", Limit: 20, Related: true, RelatedRepositoryContext: completeMatchedRepository}
			if _, err := Files(params, candidates); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for b.Loop() {
				if matches, err := Files(params, candidates); err != nil || len(matches) != 20 {
					b.Fatalf("matches=%d err=%v", len(matches), err)
				}
			}
		})
	}
}
