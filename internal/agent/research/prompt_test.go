package research

import (
	"strings"
	"testing"
)

func TestSystemPromptIncludesRootAndReadOnlyPolicy(t *testing.T) {
	prompt := SystemPrompt("/workspace/project")
	if !strings.Contains(prompt, "/workspace/project") || !strings.Contains(prompt, "read-only") || !strings.Contains(prompt, "Do not modify files") {
		t.Fatalf("prompt=%q", prompt)
	}
}
