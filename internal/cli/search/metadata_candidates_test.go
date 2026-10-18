package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	"github.com/greppleai/grepple/internal/search"
)

func metadataSearchSource(t *testing.T, root, relative, content string, file directorymeta.FileV1) directorymeta.FileV1 {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := filedigest.SHA256Hex(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = filepath.Base(path)
	file.Checksum = digest
	file.Description = "Source " + relative
	return file
}

func metadataSearchFixture(t *testing.T) string {
	t.Helper()
	root := chdirTemp(t)
	good := metadataSearchSource(t, root, "pkg/good.go", "package pkg\n// needle\nfunc Good() {}\n", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"API"}, Keywords: []string{"c++"}})
	other := metadataSearchSource(t, root, "pkg/other.go", "package pkg\n// needle\nfunc Other() {}\n", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"API"}, Keywords: []string{"c++"}})
	stale := metadataSearchSource(t, root, "pkg/stale.go", "package pkg\n// needle\nfunc Stale() {}\n", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"API"}, Keywords: []string{"c++"}})
	stale.Checksum = strings.Repeat("0", 64)
	_ = metadataSearchSource(t, root, "pkg/missing.go", "package pkg\n// needle\nfunc Missing() {}\n", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"API"}, Keywords: []string{"c++"}})
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{good, other, stale}}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMetadataSearchScopesEveryLocalOutputBeforePaging(t *testing.T) {
	metadataSearchFixture(t)
	base := []string{"--purpose", "implementation", "--domain-area", "api", "--keyword", "c++"}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"content", []string{"--line-only", "needle", "pkg"}, "pkg/good.go"},
		{"full-json", []string{"--json", "needle", "pkg"}, "pkg/good.go"},
		{"compact-json", []string{"--json-matches", "needle", "pkg"}, "pkg/good.go"},
		{"files-with-matches", []string{"--files-with-matches", "needle", "pkg"}, "pkg/good.go"},
		{"count", []string{"--count", "needle", "pkg"}, "pkg/good.go"},
		{"count-summary", []string{"--count-summary", "needle", "pkg"}, "1 files\t1 matches"},
		{"files", []string{"--files", "pkg"}, "pkg/good.go"},
		{"files-json", []string{"--files", "--json", "pkg"}, "pkg/good.go"},
		{"outline", []string{"--outline", "pkg"}, "Good"},
		{"outline-json", []string{"--outline", "--json", "pkg"}, "Good"},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := runTestSearch(append(append([]string{}, base...), test.args...)); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(out, test.want) || strings.Contains(out, "other.go") || strings.Contains(out, "stale.go") || strings.Contains(out, "missing.go") || strings.Contains(out, "Other") || strings.Contains(out, "Stale") || strings.Contains(out, "Missing") {
				t.Fatalf("unscoped %s output: %q", test.name, out)
			}
		})
	}
}

func TestMetadataSearchExplicitAtAndGlobIntersection(t *testing.T) {
	root := metadataSearchFixture(t)
	base := []string{"--purpose", "Implementation", "--domain-area", "API", "--keyword", "c++"}
	for _, name := range []string{"other", "stale", "missing"} {
		opts, _, _, err := parseTestSearchArgs(append(append([]string{}, base...), "--at", filepath.Join(root, "pkg", name+".go")+":3"))
		if err != nil {
			t.Fatal(err)
		}
		selected, err := selectLocalMetadataCandidates(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		opts.Params.MetadataCandidates = selected
		if _, err := search.At(opts.Params); err == nil || !strings.Contains(err.Error(), "does not match current local metadata") {
			t.Fatalf("at %s escaped scope: candidates=%q err=%v", name, selected, err)
		}
	}
	selected := captureStdout(t, func() {
		if err := runTestSearch(append(append([]string{}, base...), "--at", "pkg/good.go:3")); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(selected, "Good") {
		t.Fatalf("selected --at output=%q", selected)
	}
	for _, glob := range []string{"pkg/other.go", "pkg/missing.go"} {
		opts, _, _, err := parseTestSearchArgs(append(append([]string{}, base...), "needle", glob))
		if err != nil {
			t.Fatal(err)
		}
		selected, err := selectLocalMetadataCandidates(context.Background(), opts)
		if err != nil || selected == nil || len(selected) != 0 {
			t.Fatalf("glob %s candidate intersection=%q err=%v", glob, selected, err)
		}
	}
}
