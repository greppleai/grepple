package ask

import (
	"strings"
	"testing"
)

func TestResearchSystemPromptIncludesRootAndReadOnlyPolicy(t *testing.T) {
	prompt := researchSystemPrompt("/workspace/project", "")
	if !strings.Contains(prompt, "/workspace/project") || !strings.Contains(prompt, "read-only") || !strings.Contains(prompt, "Do not modify files") {
		t.Fatalf("prompt=%q", prompt)
	}
}

func TestResearchSystemPromptIncludesPreloadedTree(t *testing.T) {
	prompt := researchSystemPrompt("/workspace/project", ".\n└── parser/ — Parses source.")
	if !strings.Contains(prompt, "Preloaded local directory tree") || !strings.Contains(prompt, "parser/ — Parses source.") {
		t.Fatalf("prompt=%q", prompt)
	}
}
