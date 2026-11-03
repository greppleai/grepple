package sources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Discovery must retain only active ancestor ignore rules, not an entry for
// every directory already visited. Otherwise even empty directories turn a
// source walk into quadratic work (most visible outside initialized checkouts).
func TestCandidateWalkerBoundsIgnoreRulesToAncestors(t *testing.T) {
	for _, withRules := range []bool{false, true} {
		t.Run(fmt.Sprintf("rules=%t", withRules), func(t *testing.T) {
			root := t.TempDir()
			w := &candidateWalker{ctx: context.Background(), root: root, ignorePatterns: map[string][]string{}}
			if err := w.walkDir(root, "."); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 100; i++ {
				rel := fmt.Sprintf("dir%03d", i)
				directory := filepath.Join(root, rel)
				if err := os.Mkdir(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if withRules {
					if err := os.WriteFile(filepath.Join(directory, ".gitignore"), []byte("ignored.txt\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := w.walkDir(directory, rel); err != nil {
					t.Fatal(err)
				}
				want := 0
				if withRules {
					want = 1
				}
				if len(w.ignorePatterns) != want {
					t.Fatalf("after %s: retained %d ignore directories, want %d active ancestors", rel, len(w.ignorePatterns), want)
				}
			}
		})
	}
}

func TestCandidateWalkerPreservesAncestorAndSiblingIgnoreScopes(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{
		"a/deep/ancestor.tmp", "a/deep/local.tmp", "a/deep/keep.go",
		"b/local.tmp", "b/ancestor.tmp", "b/keep.go",
	} {
		writeScopeFile(t, root, relative)
	}
	for relative, rules := range map[string]string{
		".gitignore":   "ancestor.tmp\n",
		"a/.gitignore": "local.tmp\n",
	} {
		if err := os.WriteFile(filepath.Join(root, relative), []byte(rules), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := Listing(context.Background(), nil, DiscoveryOptions{Root: root, IgnoreRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	for _, file := range files {
		relative, _ := filepath.Rel(root, file)
		selected[filepath.ToSlash(relative)] = true
	}
	for _, path := range []string{"a/deep/keep.go", "b/keep.go", "b/local.tmp"} {
		if !selected[path] {
			t.Errorf("missing selected file %s", path)
		}
	}
	for _, path := range []string{"a/deep/ancestor.tmp", "a/deep/local.tmp", "b/ancestor.tmp"} {
		if selected[path] {
			t.Errorf("selected ignored file %s", path)
		}
	}
}

func TestDiscoveryOnlyLoadsClassificationForProductionPolicy(t *testing.T) {
	root := t.TempDir()
	if got := (DiscoveryOptions{Root: root}).ignore(); got.classifier != nil {
		t.Fatal("unfiltered source discovery loaded annotation metadata")
	}
	if got := (DiscoveryOptions{Root: root, ProductionOnly: true}).ignore(); got.classifier == nil {
		t.Fatal("production-only discovery needs a shared classifier")
	}
}

func BenchmarkListingWideUninitialized(b *testing.B) {
	for _, directories := range []int{100, 1000, 8000} {
		b.Run(fmt.Sprintf("dirs=%d", directories), func(b *testing.B) {
			root := b.TempDir()
			for i := 0; i < directories; i++ {
				directory := filepath.Join(root, fmt.Sprintf("dir%05d", i))
				if err := os.Mkdir(directory, 0700); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "source.go"), []byte("package sample\n"), 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				files, err := Listing(context.Background(), nil, DiscoveryOptions{Root: root, IgnoreRoot: root})
				if err != nil || len(files) != directories {
					b.Fatalf("selected %d files: %v", len(files), err)
				}
			}
		})
	}
}
