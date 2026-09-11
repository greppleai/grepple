package gritql

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestScanFilesFiltersAndReturnsDeterministicFindings(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	files := fstest.MapFS{
		"src/z.go":     {Data: []byte("package p\nvar x = 2\n")},
		"src/a.go":     {Data: []byte("package p\nvar x = 1\n")},
		"src/skip.go":  {Data: []byte("package p\nvar x = 3\n")},
		"src/note.txt": {Data: []byte("package p\nvar x = 4\n")},
	}
	candidates := []ScanCandidate{
		{ReadPath: "src/z.go", Path: "src/z.go"},
		{ReadPath: "src/note.txt", Path: "src/note.txt"},
		{ReadPath: "src/skip.go", Path: "src/skip.go"},
		{ReadPath: "src/a.go", Path: "src/a.go"},
	}
	result := ScanFiles(context.Background(), files, program, candidates, ScanOptions{
		PatternID: "rule", Message: "found", IncludeGlobs: []string{"**/*.go"}, ExcludeGlobs: []string{"**/skip.go"}, Workers: 2,
	})
	findings := result.Findings()
	if len(findings) != 2 || findings[0].Path() != "src/a.go" || findings[1].Path() != "src/z.go" {
		t.Fatalf("findings=%v", findings)
	}
	if diagnostics := result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}

func TestScanFilesRejectsInvalidProgramWithoutReading(t *testing.T) {
	result := ScanFiles(context.Background(), fstest.MapFS{
		"main.go": {Data: []byte("package p\nvar x = 1\n")},
	}, nil, []ScanCandidate{{ReadPath: "main.go", Path: "main.go"}}, ScanOptions{})
	if result.Stats().Evaluated != 0 || !batchHasDiagnostic(result.Diagnostics(), "INTERNAL_ERROR") {
		t.Fatalf("stats=%+v diagnostics=%v", result.Stats(), result.Diagnostics())
	}
}
