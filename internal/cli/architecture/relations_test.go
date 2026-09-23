package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchitectureDirectoryRelationsCompressesKindsByDirectoryPair(t *testing.T) {
	root := writeArchitectureRelationsFixture(t)
	chdirForConfigTest(t, root)

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--relations", "--depth", "0", "--max-nodes", "0", "--max-output-bytes", "0", "."}); err != nil {
			t.Fatal(err)
		}
	})
	const expected = "R rulespec -> search kinds=import,resolved-call,type-reference\n"
	if output != expected {
		t.Fatalf("relations output:\n%s\nwant:\n%s", output, expected)
	}
}

func TestArchitectureDirectoryMermaidWritesDiagram(t *testing.T) {
	root := writeArchitectureRelationsFixture(t)
	chdirForConfigTest(t, root)
	outputPath := filepath.Join("diagrams", "architecture.mmd")

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--mermaid", "--depth", "0", "--max-nodes", "0", "--output", outputPath, "."}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "" {
		t.Fatalf("stdout = %q", output)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	const expected = `flowchart LR
    d0["rulespec"]
    d1["search"]

    %% Link strokes use the source directory color; node borders are the legend.
    style d0 stroke:#1576d7,stroke-width:2px
    style d1 stroke:#e71742,stroke-width:2px

    d0 -->|import, resolved-call, type-reference| d1

    linkStyle 0 stroke:#1576d7,stroke-width:2px
`
	if string(content) != expected {
		t.Fatalf("Mermaid output:\n%s\nwant:\n%s", content, expected)
	}
}

func writeArchitectureRelationsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "search/request.go", "package search\ntype Request struct{}\nfunc ResolveRequest() {}\n")
	writeArchitectureFixture(t, root, "rulespec/rule.go", "package rulespec\nimport \"example.com/project/search\"\nfunc Validate(value search.Request) { search.ResolveRequest() }\n")
	return root
}
