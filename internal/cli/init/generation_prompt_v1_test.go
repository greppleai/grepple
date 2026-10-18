package initcommand

import (
	"os"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestGenerationPromptV1ReadsCurrentContextButRejectsOldSnapshot(t *testing.T) {
	root := t.TempDir()
	selected := manifestV1()
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{Name: selected[0].Name, Checksum: selected[0].Checksum, Description: "Reviewed source.", Purpose: "Implementation"}}}); err != nil {
		t.Fatal(err)
	}
	prompt, err := generationPromptV1(root, root, selected)
	if err != nil || !strings.Contains(prompt, "Reviewed source.") || !strings.Contains(prompt, "Authoritative selected file manifest") || strings.Contains(prompt, "responsibilities") == false {
		t.Fatalf("prompt: %q: %v", prompt, err)
	}
	for _, want := range []string{"documentation, implementation, testing, configuration, ci-cd, agent-skills, examples and fixture", "grounded custom purposes are allowed", "Use domainAreas for the feature or subject"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if err := os.WriteFile(directorymeta.RepositoryPath(root), []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := generationPromptV1(root, root, selected); err == nil {
		t.Fatal("accepted old snapshot as context")
	}
}
