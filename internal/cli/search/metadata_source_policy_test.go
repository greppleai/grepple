package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestMetadataSearchRejectsPipedStdinBeforeReading(t *testing.T) {
	chdirTemp(t)
	withStdin(t, "needle\n", func() {
		opts, _, _, err := parseTestSearchArgs([]string{"--keyword", "c++", "needle"})
		if err != nil {
			t.Fatal(err)
		}
		err = executeSearch(testCommandContext(), opts, "", false)
		if err == nil || !strings.Contains(err.Error(), "cannot search stdin") {
			t.Fatalf("stdin scope err=%v", err)
		}
		next := make([]byte, 7)
		if _, err := os.Stdin.Read(next); err != nil || string(next) != "needle\n" {
			t.Fatalf("stdin was consumed: %q err=%v", next, err)
		}
	})
}

func TestMetadataSearchSourcePolicyPrecedesAnnotations(t *testing.T) {
	root := chdirTemp(t)
	good := metadataSearchSource(t, root, "pkg/main.go", "package pkg\n// needle\n", directorymeta.FileV1{Purpose: "Implementation"})
	test := metadataSearchSource(t, root, "pkg/main_test.go", "package pkg\n// needle\n", directorymeta.FileV1{Purpose: "Testing"})
	ignored := metadataSearchSource(t, root, "pkg/ignored.go", "package pkg\n// needle\n", directorymeta.FileV1{Purpose: "Implementation"})
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{good, test, ignored}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("pkg/ignored.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	opts, _, _, err := parseTestSearchArgs([]string{"--purpose", "implementation", "--purpose", "testing", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	opts.Params.ProductionOnly = true
	selected, err := selectLocalMetadataCandidates(context.Background(), opts)
	if err != nil || len(selected) != 1 || filepath.Base(selected[0]) != "main.go" {
		t.Fatalf("production + gitignore candidates=%q err=%v", selected, err)
	}
}
