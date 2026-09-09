package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type exampleSymbol struct {
	kind string
	name string
}

type advancedExample struct {
	file             string
	language         string
	allowsCompaction bool
	symbols          []exampleSymbol
}

var advancedExamples = []advancedExample{
	{file: "gateway.go", language: "go", symbols: []exampleSymbol{{"interface", "Auditor"}, {"method", "(*Gateway[T]).Deliver"}}},
	{file: "pipeline.js", language: "javascript", symbols: []exampleSymbol{{"class", "Pipeline"}, {"method", "run"}}},
	{file: "SearchPanel.tsx", language: "tsx", allowsCompaction: true, symbols: []exampleSymbol{{"type", "SearchPanelProps"}, {"function", "SearchPanel"}}},
	{file: "worker.py", language: "python", symbols: []exampleSymbol{{"class", "Worker"}, {"function", "process"}}},
	{file: "Workflow.java", language: "java", symbols: []exampleSymbol{{"interface", "Event"}, {"method", "execute"}}},
	{file: "Scheduler.kt", language: "kotlin", symbols: []exampleSymbol{{"interface", "ScheduleResult"}, {"fun", "schedule"}}},
	{file: "Catalog.cs", language: "csharp", symbols: []exampleSymbol{{"namespace", "Advanced.Catalog"}, {"method", "BuildSnapshot"}}},
	{file: "cache.c", language: "c", symbols: []exampleSymbol{{"typedef", "revision_cache"}, {"function", "cache_put"}}},
	{file: "index.cpp", language: "cpp", symbols: []exampleSymbol{{"concept", "Revisioned"}, {"method", "lookup"}}},
	{file: "worker.rs", language: "rust", symbols: []exampleSymbol{{"trait", "Executor"}, {"function", "drain"}}},
	{file: "deploy.sh", language: "shell", symbols: []exampleSymbol{{"function", "wait_for_rollout"}, {"function", "main"}}},
}

func TestAdvancedExamplesHaveUsefulOutlines(t *testing.T) {
	for _, example := range advancedExamples {
		t.Run(example.file, func(t *testing.T) {
			path, content := readAdvancedExample(t, example.file)
			outline := OutlineFile(path, content)
			if outline.Language != example.language {
				t.Fatalf("language = %q, want %q", outline.Language, example.language)
			}
			for _, symbol := range example.symbols {
				mustFind(t, outline.Symbols, symbol.kind, symbol.name)
			}
		})
	}
}

func TestAdvancedExamplesParseWithoutErrors(t *testing.T) {
	for _, example := range advancedExamples {
		t.Run(example.file, func(t *testing.T) {
			_, content := readAdvancedExample(t, example.file)
			tree, err := parseTree(adapterForLanguage(example.language), content)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			defer tree.Close()
			if tree.RootNode().HasError() {
				t.Fatal("fixture contains tree-sitter parse errors")
			}
		})
	}
}

func TestAdvancedExamplesReturnDocumentedDeclaration(t *testing.T) {
	for _, example := range advancedExamples {
		t.Run(example.file, func(t *testing.T) {
			assertAdvancedExampleSegments(t, example)
		})
	}
}

func assertAdvancedExampleSegments(t *testing.T, example advancedExample) {
	t.Helper()
	_, content := readAdvancedExample(t, example.file)
	docLine := markerLine(t, content, "ADVANCED_DOC")
	bodyLine := markerLine(t, content, "ADVANCED_END")
	segments := BuildSegments(content, example.language, map[int]bool{bodyLine: true}, 64)
	covered := coveredSegmentLines(segments)

	if !covered[docLine] || !covered[bodyLine] {
		t.Fatalf("documentation line %d and matched body line %d must be present in segments %#v", docLine, bodyLine, segments)
	}
	if example.allowsCompaction {
		return
	}
	for line := docLine; line <= bodyLine; line++ {
		if !covered[line] {
			t.Fatalf("documented declaration line %d is missing from segments %#v", line, segments)
		}
	}
}

func coveredSegmentLines(segments []Segment) map[int]bool {
	covered := make(map[int]bool)
	for _, segment := range segments {
		if segment.Kind != "lines" {
			continue
		}
		for line := segment.Start; line <= segment.End; line++ {
			covered[line] = true
		}
	}
	return covered
}

func readAdvancedExample(t *testing.T, file string) (string, string) {
	t.Helper()
	path := filepath.Join("..", "examples", "advanced-files", file)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return path, string(content)
}

func markerLine(t *testing.T, content, marker string) int {
	t.Helper()
	for index, line := range strings.Split(content, "\n") {
		if strings.Contains(line, marker) {
			return index + 1
		}
	}
	t.Fatalf("marker %q not found", marker)
	return 0
}
