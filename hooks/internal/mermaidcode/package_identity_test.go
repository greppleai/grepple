package mermaidcode

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoPackageIdentityIncludesDirectory(t *testing.T) {
	root := t.TempDir()
	first := Source{filepath.Join(root, "one", "item.go"), "package shared\ntype Item struct { One string }\n"}
	second := Source{filepath.Join(root, "two", "item.go"), "package shared\ntype Item struct { Two string }\n"}
	analysis, err := Analyze([]Source{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.GoDeclarations) != 2 {
		t.Fatalf("declarations were overwritten: %v", analysis.GoDeclarations)
	}
	if analysis.Declarations["Item"] != nil {
		t.Fatal("ambiguous generic declaration retained a path-dependent value")
	}
	generated, err := GenerateClassDiagram("Item", first, []Source{first, second}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated, "Two") {
		t.Fatalf("generation combined package directories: %s", generated)
	}
}
