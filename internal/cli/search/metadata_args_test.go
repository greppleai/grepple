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

func TestMetadataSearchFlagsParseNormalizedExactAndRejectUnsafeScopes(t *testing.T) {
	options, _, remote, err := parseTestSearchArgs([]string{"--purpose", "Testing", "--purpose", "Implementation", "--domain-area", "API", "--keyword=c++", "needle", "pkg"})
	if err != nil || remote || options == nil || !options.MetadataSelector.Requested() {
		t.Fatalf("selector=%+v remote=%t err=%v", options, remote, err)
	}
	if !options.MetadataSelector.Matches(directorymeta.FileStateV1{Status: directorymeta.StatusCurrent, File: directorymeta.FileV1{Purpose: "implementation", DomainAreas: []string{"api"}, Keywords: []string{"c++"}}}) || options.MetadataSelector.Matches(directorymeta.FileStateV1{Status: directorymeta.StatusStale, File: directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"API"}, Keywords: []string{"c++"}}}) {
		t.Fatal("parsed selector broke OR/AND or current-file constraint")
	}
	if options.Params.Related || options.Params.FollowRelated != 0 {
		t.Fatal("implicit related navigation must not expand filtered searches")
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--purpose", "Testing", "--remote", "needle"}, "--remote"},
		{[]string{"--purpose", "Testing", "--remote-only", "needle"}, "--remote-only"},
		{[]string{"--purpose", "Testing", "--server", "https://example.test", "needle"}, "--server"},
		{[]string{"--purpose", "Testing", "--cursor", "token", "needle"}, "--cursor"},
		{[]string{"--purpose", "Testing", "--repo", "owner/repo", "needle"}, "--repo"},
		{[]string{"--purpose", "Testing", "--related", "needle"}, "--related"},
		{[]string{"--purpose", "Testing", "--follow-related", "2", "needle"}, "--follow-related"},
		{[]string{"--keyword", "", "needle"}, "--keyword"},
	} {
		if _, _, _, err := parseTestSearchArgs(test.args); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("args=%q err=%v want=%q", test.args, err, test.want)
		}
	}
}

func TestMetadataSearchMissingAnnotationsCannotSearchUnfiltered(t *testing.T) {
	root := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\n// needle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	opts, _, _, err := parseTestSearchArgs([]string{"--keyword", "c++", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := selectLocalMetadataCandidates(context.Background(), opts)
	if err != nil || candidates == nil || len(candidates) != 0 {
		t.Fatalf("missing metadata candidates=%q err=%v", candidates, err)
	}
	opts.Params.MetadataCandidates = candidates
	matches, err := search.Files(opts.Params, nil)
	if err != nil || len(matches) != 0 {
		t.Fatalf("unfiltered fallback: matches=%+v err=%v", matches, err)
	}
}
