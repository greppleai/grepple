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

func TestFreshInitPromptRequestsEvidenceBackedAreaDiscovery(t *testing.T) {
	root := t.TempDir()
	prompt, err := generationPromptWithAreas(root, root, []directorymeta.File{{Path: "example.go", Checksum: "hash"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"(none)", "Independently discover cohesive feature areas", "based on evidence, not a preset vocabulary", "area_proposals entry", "example.go:12", "Never use the literal placeholder SOURCE:LINE", "If no meaningful feature area is evidenced, leave areas empty"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("missing %q in prompt: %s", required, prompt)
		}
	}
	if system := initSystemPrompt(root, root); !strings.Contains(system, "including when there are no existing area tags") || !strings.Contains(system, "Do not write SOURCE:LINE") {
		t.Fatal("system prompt must request fresh discovery")
	}
}

func TestAreaPromptsReuseNewTagsOnlyInSequentialMode(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, relative := range []string{"a/a.go", "b/b.go"} {
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &bytes.Buffer{}, ErrorOutput: &bytes.Buffer{}})
	firstDir := filepath.Join(root, "a")
	firstFiles, err := directorymeta.FilesForDirectory(root, firstDir, []string{"a/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	secondDir := filepath.Join(root, "b")
	secondFiles, err := directorymeta.FilesForDirectory(root, secondDir, []string{"b/b.go"})
	if err != nil {
		t.Fatal(err)
	}
	firstJob := generationJob{directory: firstDir, files: firstFiles}
	secondJob := generationJob{directory: secondDir, files: secondFiles}
	sequential := areaPrompts(application, root, 1, nil)
	parallel := areaPrompts(application, root, 2, nil)
	firstPrompt, err := sequential(context.Background(), firstJob)
	if err != nil || !strings.Contains(firstPrompt, "(none)") {
		t.Fatalf("initial prompt=%q err=%v", firstPrompt, err)
	}
	firstFiles[0].Description, firstFiles[0].Kind, firstFiles[0].Areas = "Handles checkout.", "production", []string{"checkout"}
	if err := directorymeta.Write(firstDir, directorymeta.Metadata{Description: "Checkout.", Responsibilities: []string{"Run checkout."}, Files: firstFiles}); err != nil {
		t.Fatal(err)
	}
	secondPrompt, err := sequential(context.Background(), secondJob)
	if err != nil || !strings.Contains(secondPrompt, "checkout a/a.go") {
		t.Fatalf("sequential prompt=%q err=%v", secondPrompt, err)
	}
	parallelPrompt, err := parallel(context.Background(), secondJob)
	if err != nil || strings.Contains(parallelPrompt, "checkout a/a.go") {
		t.Fatalf("parallel prompt=%q err=%v", parallelPrompt, err)
	}
}
