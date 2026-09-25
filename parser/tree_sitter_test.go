package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withRepoRoot(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(old, ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestTreeSitterLanguageParity(t *testing.T) {
	withRepoRoot(t)
	tests := []struct {
		language string
		path     string
		query    string
		nodeKind string
	}{
		{"typescript", "testdata/typescript/sample.ts", "formatUser", "function_declaration"},
		{"tsx", "testdata/typescript/large-component.tsx", "data-testid", "function_declaration"},
		{"javascript", "testdata/javascript/sample.js", "console.log", "class_declaration"},
		{"go", "testdata/go/sample.go", "FormatUser", "function_declaration"},
		{"kotlin", "testdata/kotlin/Sample.kt", "println", "class_declaration"},
		{"java", "testdata/java/Sample.java", "System.out.println", "class_declaration"},
		{"php", "testdata/php/sample.php", "Format::render", "class_declaration"},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			verifyLanguageParity(t, test.language, test.path, test.query, test.nodeKind)
		})
	}
}

func verifyLanguageParity(t *testing.T, language, path, query, nodeKind string) {
	t.Helper()
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(contentBytes)
	config := adapterForLanguage(language)
	tree, err := config.Parse(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	found := false
	tree.RootNode().WalkNamed(func(node *syntaxNode) {
		if node.Kind() == nodeKind {
			found = true
		}
	})
	tree.Close()
	if !found {
		t.Fatalf("missing AST node kind %q", nodeKind)
	}

	hits := map[int]bool{}
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, query) {
			hits[i+1] = true
		}
	}
	if len(hits) == 0 {
		t.Fatalf("fixture has no %q hit", query)
	}
	for _, segment := range BuildSegments(content, language, hits) {
		if segment.Kind == "lines" && segment.Start > segment.End {
			t.Fatalf("invalid segment: %#v", segment)
		}
	}
}

func TestBuildSegmentsParsesSupportedSourceOnce(t *testing.T) {
	parseInvocations.Store(0)
	segments := BuildSegments("package p\nfunc Target() {}\n", "go", map[int]bool{2: true})
	if len(segments) == 0 {
		t.Fatal("expected segments")
	}
	if got := parseInvocations.Load(); got != 1 {
		t.Fatalf("BuildSegments parsed %d times, want 1", got)
	}
}

func TestBuildSegmentsReportsStructuralStatus(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		language string
		want     SegmentBuildStatus
	}{
		{"structured", "package p\nfunc Target() {}\n", "go", SegmentBuildStructured},
		{"recovered", "package p\nfunc Target( {\n", "go", SegmentBuildRecovered},
		{"plain", "target\n", "text", SegmentBuildPlain},
		{"unsupported", "target\n", "unknown", SegmentBuildUnsupported},
		{"invalid UTF-8", string([]byte{0xff}), "go", SegmentBuildFailed},
	}
	for _, test := range tests {
		_, status := BuildSegmentsWithStatus(test.content, test.language, map[int]bool{1: true})
		if status != test.want {
			t.Errorf("%s: status=%q, want %q", test.name, status, test.want)
		}
	}
}
