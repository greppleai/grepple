package initcommand

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestParseGeneratedMetadataPreservesPathsAndChecksums(t *testing.T) {
	files := []directorymeta.File{{Path: "file.go", Checksum: "abc"}}
	metadata, err := parseGeneratedMetadata("description: Parses files.\nresponsibilities:\n- Parse files.\nfiles:\n- path: file.go\n  description: Contains parsing logic.\n  kind: production\n  checksum: ignored\n", files)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Files[0].Path != "file.go" || metadata.Files[0].Checksum != "abc" || metadata.Files[0].Description != "Contains parsing logic." || metadata.Files[0].Kind != "production" {
		t.Fatalf("metadata=%+v", metadata)
	}
}
func TestParseGeneratedMetadataRequiresValidSourceKind(t *testing.T) {
	files := []directorymeta.File{{Path: "file.go", Checksum: "abc"}}
	for _, test := range []struct {
		name string
		kind string
	}{
		{name: "missing"},
		{name: "invalid", kind: "application"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := "description: Parses files.\nresponsibilities: [Parse files.]\nfiles:\n- path: file.go\n  description: Contains parsing logic.\n"
			if test.kind != "" {
				value += "  kind: " + test.kind + "\n"
			}
			if _, err := parseGeneratedMetadata(value, files); err == nil {
				t.Fatal("invalid source kind succeeded")
			}
		})
	}
}

func TestGenerationPromptPrimesAgentWithExistingMetadata(t *testing.T) {
	directory := t.TempDir()
	existing := "description: Existing architectural context.\nresponsibilities: [Own behavior.]\nfiles: []\n"
	if err := os.WriteFile(filepath.Join(directory, directorymeta.FileName), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt, err := generationPrompt(directory, directory, []directorymeta.File{{Path: "file.go", Checksum: "abc"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, existing) || !strings.Contains(prompt, "Authoritative file manifest") {
		t.Fatalf("prompt=%q", prompt)
	}
}

func TestInitToolsReadGrepAndTreeWithinCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "pkg")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("pkg/ignored.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package pkg\n\nfunc Needle() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ignored.go"), []byte("package pkg\nfunc Ignored() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}})

	read, err := runInitRead(context.Background(), directory, initReadInput{Path: "main.go", StartLine: 3, EndLine: 3})
	if err != nil || !strings.Contains(read, "3│func Needle() {}") {
		t.Fatalf("read=%q err=%v", read, err)
	}
	grep, err := runInitGrep(context.Background(), application, root, directory, initGrepInput{Query: "Needle"})
	if err != nil || !strings.Contains(grep, "main.go:3│func Needle() {}") {
		t.Fatalf("grep=%q err=%v", grep, err)
	}
	tree, err := runInitTree(context.Background(), application, root, directory, initTreeInput{Depth: 2})
	if err != nil || !strings.Contains(tree, "main.go") || strings.Contains(tree, "ignored.go") {
		t.Fatalf("tree=%q err=%v", tree, err)
	}
	if _, err := runInitRead(context.Background(), directory, initReadInput{Path: "../outside.go"}); err == nil {
		t.Fatal("expected path confinement error")
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "linked.go")); err == nil {
		if _, err := runInitRead(context.Background(), directory, initReadInput{Path: "linked.go"}); err == nil {
			t.Fatal("expected symlink confinement error")
		}
	}
}

func TestConfinedDirectorySupportsExactlyOneDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "pkg")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := confinedDirectory(root, "pkg")
	if err != nil || resolved != directory {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	if _, err := confinedDirectory(root, "../outside"); err == nil {
		t.Fatal("expected repository confinement error")
	}
}
