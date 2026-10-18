package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/search"
)

func TestMetadataSearchMalformedAndAbsentAnnotationsStayEmpty(t *testing.T) {
	root := metadataSearchFixture(t)
	metadata := filepath.Join(root, filepath.FromSlash(directorymeta.RepositoryFile))
	for _, test := range []struct {
		name   string
		write  string
		remove bool
	}{
		{"malformed", "schema: v1\ndirectory: []\n", false},
		{"absent", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.remove {
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(metadata, []byte(test.write), 0600); err != nil {
				t.Fatal(err)
			}
			for _, listing := range []bool{false, true} {
				args := []string{"--purpose", "Implementation", "needle", "pkg"}
				if listing {
					args = []string{"--purpose", "Implementation", "--files", "pkg"}
				}
				opts, _, _, err := parseTestSearchArgs(args)
				if err != nil {
					t.Fatal(err)
				}
				candidates, err := selectLocalMetadataCandidates(context.Background(), opts)
				if err != nil || candidates == nil || len(candidates) != 0 {
					t.Fatalf("listing=%v candidates=%q err=%v", listing, candidates, err)
				}
				opts.Params.MetadataCandidates = candidates
				if listing {
					paths, err := search.ListFilePaths(opts.Params, nil)
					if err != nil || len(paths) != 0 {
						t.Fatalf("leaked listing: %q %v", paths, err)
					}
				} else {
					matches, err := search.Files(opts.Params, nil)
					if err != nil || len(matches) != 0 {
						t.Fatalf("leaked content: %+v %v", matches, err)
					}
				}
			}
		})
	}
}

func TestMetadataSearchPaginationFiltersBeforeWindow(t *testing.T) {
	metadataSearchFixture(t)
	opts, _, _, err := parseTestSearchArgs([]string{"--purpose", "Implementation", "--keyword", "c++", "--limit", "1", "--sort", "matches", "needle", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selectLocalMetadataCandidates(context.Background(), opts)
	if err != nil || len(selected) != 1 || !strings.HasSuffix(selected[0], "good.go") {
		t.Fatalf("candidate window=%q err=%v", selected, err)
	}
	opts.Params.MetadataCandidates = selected
	matches, err := search.Files(opts.Params, nil)
	if err != nil || len(matches) != 1 || !strings.HasSuffix(matches[0].File, "good.go") {
		t.Fatalf("result window=%+v err=%v", matches, err)
	}
	opts.Params.Skip = 1
	matches, err = search.Files(opts.Params, nil)
	if err != nil || len(matches) != 0 {
		t.Fatalf("skip escaped scope=%+v err=%v", matches, err)
	}
}
