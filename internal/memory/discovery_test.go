package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func putMemory(t *testing.T, root, name string, files []Attachment, domain, functional, keywords []string) {
	t.Helper()
	h := Header{Schema: "grepple-memory-v1", Name: name, Description: "A safe short hint", Files: files, DomainAreas: domain, FunctionalAreas: functional, Keywords: keywords}
	if err := Save(root, h, []byte("private words that must not be matched\n"), false); err != nil {
		t.Fatal(err)
	}
}

func putScopedMemory(t *testing.T, root, name string, scopes []string, files []Attachment) {
	t.Helper()
	h := Header{Schema: "grepple-memory-v1", Name: name, Description: "A safe short hint", Scopes: scopes, Files: files}
	if err := Save(root, h, []byte("private words that must not be matched\n"), false); err != nil {
		t.Fatal(err)
		t.Fatal(err)
	}
}

func putMetadata(t *testing.T, root string, entries map[string][]directorymeta.FileV1) {
	t.Helper()
	d := directorymeta.DocumentV1{Schema: "v1", Directory: map[string]directorymeta.DirectoryV1{}}
	for key, files := range entries {
		d.Directory[key] = directorymeta.DirectoryV1{Files: files}
	}
	if err := directorymeta.WriteDocumentV1(root, d); err != nil {
		t.Fatal(err)
	}
}
func TestDiscoverRanksCurrentEvidenceAndOnlyMatchedPage(t *testing.T) {
	root := fixture(t)
	_, digest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	pathOnly := Attachment{Path: "a.go", Checksum: digest}
	putMemory(t, root, "whole", []Attachment{pathOnly}, nil, nil, nil)
	putMemory(t, root, "range", []Attachment{{Path: "a.go", Checksum: digest, LineRange: []int{2, 2}}}, nil, nil, nil)
	putMemory(t, root, "symbol", []Attachment{{Path: "a.go", Checksum: digest, Symbols: []string{"@func:2-2"}}}, nil, nil, nil)
	putMemory(t, root, "labels", nil, []string{"Alpha"}, []string{"Beta"}, []string{"Gamma"})
	putMemory(t, root, "unrelated", nil, []string{"elsewhere"}, nil, nil)
	putMetadata(t, root, map[string][]directorymeta.FileV1{".": {{Name: "a.go", Checksum: digest, Description: "A file", Purpose: "implementation", DomainAreas: []string{"alpha"}, FunctionalAreas: []string{"beta"}, Keywords: []string{"gamma"}}}})
	page := []MatchedFile{{Path: "a.go", Lines: []int{2}}}
	got := Discover(root, page)
	if !got.Complete || got.Omitted != 1 || len(got.References) != 3 {
		t.Fatalf("discovery %+v", got)
	}
	for index, name := range []string{"symbol", "range", "whole"} {
		if got.References[index].Name != name {
			t.Fatalf("rank %+v", got)
		}
	}
	if got.References[0].Description != "A safe short hint" {
		t.Fatalf("description %+v", got)
	}
	if got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{1}}}); len(got.References) != 2 || got.References[0].Name != "whole" || got.References[1].Name != "labels" {
		t.Fatalf("only matched line: %+v", got)
	}
	if got := Discover(root, []MatchedFile{{Path: "a.go", Spans: []LineSpan{{Start: 1, End: 2}}}}); len(got.References) != 3 || got.References[0].Name != "symbol" {
		t.Fatalf("multiline %+v", got)
	}
	if got := Discover(root, []MatchedFile{{Path: "../outside", Lines: []int{2}}}); len(got.References) != 0 {
		t.Fatalf("escaped path %+v", got)
	}
	if got := Discover(root, nil); len(got.References) != 0 || !got.Complete {
		t.Fatalf("no results %+v", got)
	}
}
func TestDiscoverScopeEvidenceUsesOnlySelectedPaths(t *testing.T) {
	root := fixture(t)
	for name := range map[string]string{
		"internal/cli/direct.go":       "package cli\nfunc Direct() {}\n",
		"internal/cli/nested/deep.go":  "package nested\nfunc Deep() {}\n",
		"internal/other/unselected.go": "package other\nfunc Other() {}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("package test\nfunc Run() {}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	putScopedMemory(t, root, "directory", []string{"internal/cli/"}, nil)
	putScopedMemory(t, root, "direct", []string{"internal/cli/*.go"}, nil)
	putScopedMemory(t, root, "recursive", []string{"internal/cli/**/*.go"}, nil)

	page := []MatchedFile{{Path: "internal/cli/direct.go", Lines: []int{2}}}
	got := Discover(root, page)
	if !got.Complete || names(got) != "direct,directory,recursive" {
		t.Fatalf("direct scope discovery %+v", got)
	}
	got = Discover(root, []MatchedFile{{Path: "internal/cli/nested/deep.go", Lines: []int{2}}})
	if !got.Complete || names(got) != "directory,recursive" {
		t.Fatalf("recursive scope discovery %+v", got)
	}
	got = Discover(root, []MatchedFile{{Path: "internal/other/unselected.go", Lines: []int{2}}})
	if !got.Complete || len(got.References) != 0 {
		t.Fatalf("unselected directory scope leaked %+v", got)
	}
	if got = Discover(root, []MatchedFile{{Path: "internal/cli/direct.go", Lines: nil}}); len(got.References) != 0 {
		t.Fatalf("scope accepted no actual match evidence %+v", got)
	}
	if got = Discover(root, []MatchedFile{{Path: "../internal/cli/direct.go", Lines: []int{2}}}); len(got.References) != 0 {
		t.Fatalf("scope accepted escaped path %+v", got)
	}
	if got = Discover(root, []MatchedFile{{Path: "internal/cli/direct.go", Lines: []int{99}}}); len(got.References) != 0 {
		t.Fatalf("scope accepted invalid match line %+v", got)
	}
	if got = Discover(root, []MatchedFile{{Path: "internal/cli/direct.go", Spans: []LineSpan{{Start: 1, End: 2}}}}); names(got) != "direct,directory,recursive" {
		t.Fatalf("scope rejected actual span evidence %+v", got)
	}
	putScopedMemory(t, root, "stale", []string{"internal/cli/"}, []Attachment{{Path: "a.go", Checksum: strings.Repeat("0", 64)}})
	_, digest, err := Source(root, "internal/cli/direct.go")
	if err != nil {
		t.Fatal(err)
	}
	putMemory(t, root, "labels", nil, []string{"alpha"}, nil, nil)
	putMetadata(t, root, map[string][]directorymeta.FileV1{"internal/cli": {{Name: "direct.go", Checksum: digest, Description: "Direct", Purpose: "implementation", DomainAreas: []string{"alpha"}}}})
	got = Discover(root, page)
	if names(got) != "labels,direct,directory" || got.Omitted != 2 {
		t.Fatalf("label and stale scope ranking %+v", got)
	}
}

func names(result Discovery) string {
	values := make([]string, 0, len(result.References))
	for _, reference := range result.References {
		values = append(values, reference.Name)
	}
	return strings.Join(values, ",")
}

func TestDiscoverStaleAttachmentsAndFileOwnedLabels(t *testing.T) {
	root := fixture(t)
	_, old, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	putMemory(t, root, "attached", []Attachment{{Path: "a.go", Checksum: old}}, nil, nil, nil)
	putMemory(t, root, "labeled", nil, []string{"alpha"}, []string{"beta"}, nil)
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package a\nfunc Other() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, other, err := Source(root, "b.go")
	if err != nil {
		t.Fatal(err)
	}
	putMetadata(t, root, map[string][]directorymeta.FileV1{".": {
		{Name: "a.go", Checksum: old, Description: "A file", Purpose: "implementation", DomainAreas: []string{"alpha"}},
		{Name: "b.go", Checksum: other, Description: "B file", Purpose: "implementation", FunctionalAreas: []string{"beta"}},
	}})
	page := []MatchedFile{{Path: "a.go", Lines: []int{2}}, {Path: "b.go", Lines: []int{2}}}
	got := Discover(root, page)
	if len(got.References) != 1 || got.References[0].Name != "attached" {
		t.Fatalf("cross-file conjunction %+v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = Discover(root, page)
	if len(got.References) != 0 || !got.Complete {
		t.Fatalf("stale source/metadata %+v", got)
	}
	_, newDigest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	putMetadata(t, root, map[string][]directorymeta.FileV1{".": {
		{Name: "a.go", Checksum: newDigest, Description: "A file", Purpose: "implementation", DomainAreas: []string{"alpha"}, FunctionalAreas: []string{"beta"}},
		{Name: "b.go", Checksum: other, Description: "B file", Purpose: "implementation"},
	}})
	got = Discover(root, page)
	if len(got.References) != 1 || got.References[0].Name != "labeled" {
		t.Fatalf("fresh same-file labels %+v", got)
	}
	if got = Discover(root, []MatchedFile{{Path: "b.go", Lines: []int{2}}}); len(got.References) != 0 {
		t.Fatalf("unrelated selected file %+v", got)
	}
}
func TestDiscoverAbsentInvalidAndLimits(t *testing.T) {
	root := fixture(t)
	if got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{2}}}); !got.Complete || len(got.References) != 0 {
		t.Fatalf("missing directory %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".grepple")); !os.IsNotExist(err) {
		t.Fatalf("discovery created directory: %v", err)
	}
	_, digest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	putMemory(t, root, "valid", []Attachment{{Path: "a.go", Checksum: digest}}, nil, nil, nil)
	dir := filepath.Join(root, ".grepple", "memories")
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\ninvalid: true\n---\nprivate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "valid.md"), filepath.Join(dir, "symlink.md")); err != nil {
		t.Fatal(err)
	}
	got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{2}}})
	if got.Complete || len(got.References) != 1 || got.References[0].Name != "valid" {
		t.Fatalf("invalid entries %+v", got)
	}
	if err := os.Remove(filepath.Join(dir, "bad.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "symlink.md")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < scanLimit; i++ {
		name := filepath.Join(dir, "x"+strings.Repeat("0", 3)+string(rune(0x1000+i))+".txt")
		if err := os.WriteFile(name, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got = Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{2}}})
	if got.Complete {
		t.Fatalf("scan budget %+v", got)
	}
}

func TestDiscoverSymbolReverificationAndBounds(t *testing.T) {
	root := fixture(t)
	_, digest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	h := Header{Schema: "grepple-memory-v1", Name: "symbol", Description: "Short", Files: []Attachment{{Path: "a.go", Checksum: digest, Symbols: []string{"@func:2-2"}}}}
	if err := Save(root, h, []byte("private"), false); err != nil {
		t.Fatal(err)
	}
	page := []MatchedFile{{Path: "a.go", Lines: []int{2}}}
	if got := Discover(root, page); len(got.References) != 1 {
		t.Fatalf("initial symbol %+v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nvar X = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, digest, err = Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	h.Files[0].Checksum = digest
	// A copied selector with a deliberately refreshed checksum must still resolve.
	if err := Save(root, h, []byte("private"), true); err != nil {
		t.Fatal(err)
	}
	if got := Discover(root, page); len(got.References) != 0 {
		t.Fatalf("stale symbol matched %+v", got)
	}
	if got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{900}}}); len(got.References) != 0 {
		t.Fatalf("out-of-range match %+v", got)
	}
	if got := Discover(root, []MatchedFile{{Path: "a.go", Spans: []LineSpan{{Start: 2, End: 900}}}}); len(got.References) != 0 {
		t.Fatalf("invalid span %+v", got)
	}
}

func TestDiscoverNoAncestorUnionOrBodyParsing(t *testing.T) {
	root := fixture(t)
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.go"), []byte("package nested\nfunc B() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, first, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := Source(root, "nested/b.go")
	if err != nil {
		t.Fatal(err)
	}
	putMemory(t, root, "label", nil, []string{"alpha"}, nil, nil)
	putMetadata(t, root, map[string][]directorymeta.FileV1{
		".":      {{Name: "a.go", Checksum: first, Description: "A file", Purpose: "implementation", DomainAreas: []string{"alpha"}}},
		"nested": {{Name: "b.go", Checksum: second, Description: "B file", Purpose: "implementation", DomainAreas: []string{"beta"}}},
	})
	if got := Discover(root, []MatchedFile{{Path: "nested/b.go", Lines: []int{2}}}); len(got.References) != 0 {
		t.Fatalf("ancestor label leaked %+v", got)
	}
	name := filepath.Join(root, ".grepple", "memories", "label.md")
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	index := strings.Index(string(raw), "\n---\n") + 5
	if index < 5 {
		t.Fatal("missing delimiter")
	}
	raw = append(raw[:index], 0xff, 0x00)
	if err := os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{2}}}); len(got.References) != 1 {
		t.Fatalf("body was parsed or read %+v", got)
	}
	if _, _, err := Recall(root, "label"); err == nil {
		t.Fatal("explicit recall should reject invalid body")
	}
}

func TestDiscoverOversizedEntriesAndMetadata(t *testing.T) {
	root := fixture(t)
	_, digest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	putMemory(t, root, "path", []Attachment{{Path: "a.go", Checksum: digest}}, nil, nil, nil)
	putMemory(t, root, "label", nil, []string{"alpha"}, nil, nil)
	dir := filepath.Join(root, ".grepple", "memories")
	// Even a valid header cannot qualify when its entire entry is oversized.
	raw, err := os.ReadFile(filepath.Join(dir, "path.md"))
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte(strings.Repeat("x", EntryLimit))...)
	if err := os.WriteFile(filepath.Join(dir, "path.md"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(root, ".grepple", "grepple.yaml")
	if err := os.WriteFile(metadata, []byte(strings.Repeat("x", metadataBytesLimit+1)), 0600); err != nil {
		t.Fatal(err)
	}
	got := Discover(root, []MatchedFile{{Path: "a.go", Lines: []int{2}}})
	if got.Complete || len(got.References) != 0 || got.Omitted != 0 {
		t.Fatalf("oversized inputs %+v", got)
	}
}
