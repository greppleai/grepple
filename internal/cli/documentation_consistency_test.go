package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapsSeparatePendingAndCompletedItems(t *testing.T) {
	next := readRepositoryDocument(t, "next.md")
	solved := readRepositoryDocument(t, "solved.md")
	if strings.Contains(next, "- [x]") {
		t.Fatal("next.md contains completed checklist items; move them to solved.md")
	}
	if strings.Contains(solved, "- [ ]") {
		t.Fatal("solved.md contains pending checklist items; move them to next.md")
	}
	pending := roadmapChecklistItems(next)
	for item := range roadmapChecklistItems(solved) {
		if pending[item] {
			t.Fatalf("roadmap item appears in both next.md and solved.md: %q", item)
		}
	}
}

func TestOutputContractDocumentationRemainsDiscoverableAndCurrent(t *testing.T) {
	readme := readRepositoryDocument(t, "README.md")
	contract := readRepositoryDocument(t, filepath.Join("docs", "output-contracts.md"))
	analysis := readRepositoryDocument(t, filepath.Join("docs", "gritql-analysis.md"))
	if !strings.Contains(readme, "docs/output-contracts.md") {
		t.Fatal("README.md does not link to docs/output-contracts.md")
	}
	for _, heading := range []string{"## Command output modes", "## Limit units", "## Local and remote availability", "## Resolution outcomes and confidence"} {
		if !strings.Contains(contract, heading) {
			t.Fatalf("output contract is missing %q", heading)
		}
	}
	if strings.Contains(analysis, "Implement the native `gritql-go-v1` detection kernel") {
		t.Fatal("docs/gritql-analysis.md still recommends an already completed implementation milestone")
	}
}

func readRepositoryDocument(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", path))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func roadmapChecklistItems(document string) map[string]bool {
	items := map[string]bool{}
	for _, line := range strings.Split(document, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [ ] ") && !strings.HasPrefix(line, "- [x] ") {
			continue
		}
		items[strings.TrimSpace(line[6:])] = true
	}
	return items
}
