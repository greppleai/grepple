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
func TestScanFilesUsesPreacquiredContentWithoutReading(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	source := []byte("package p\nvar x = 1\n")
	result := ScanFiles(context.Background(), nil, program, []ScanCandidate{{
		ReadPath: "missing.go", Path: "main.go", Content: source,
	}}, ScanOptions{})
	if findings := result.Findings(); len(findings) != 1 || findings[0].Path() != "main.go" {
		t.Fatalf("findings=%v diagnostics=%v", findings, result.Diagnostics())
	}
	if result.Stats().BytesRead != int64(len(source)) {
		t.Fatalf("stats=%+v", result.Stats())
	}
}
func TestScanFilesAppliesMandatoryAnchorBeforeParsing(t *testing.T) {
	program := compileFindingPattern(t, "`target($value)`")
	files := fstest.MapFS{
		"match.go": {Data: []byte("package p\nvar _ = target(value)\n")},
		"skip.go":  {Data: []byte("package p\nfunc broken(\n")},
	}
	result := ScanFiles(context.Background(), files, program, scanCandidates("match.go", "skip.go"), ScanOptions{Workers: 1})
	if findings := result.Findings(); len(findings) != 1 || findings[0].Path() != "match.go" {
		t.Fatalf("findings=%v diagnostics=%v", findings, result.Diagnostics())
	}
	if stats := result.Stats(); stats.Evaluated != 1 || stats.SkippedAnchor != 1 {
		t.Fatalf("stats=%+v", stats)
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
func TestScanFilesDeduplicationPrefersPreacquiredContent(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	source := []byte("package p\nvar x = 1\n")
	result := ScanFiles(context.Background(), nil, program, []ScanCandidate{
		{ReadPath: "main.go", Path: "main.go"},
		{ReadPath: "main.go", Path: "main.go", Content: source},
	}, ScanOptions{})
	if findings := result.Findings(); len(findings) != 1 || result.Stats().Eligible != 1 {
		t.Fatalf("findings=%v stats=%+v diagnostics=%v", findings, result.Stats(), result.Diagnostics())
	}
}
func TestScanFilesAppliesSourceLimitToPreacquiredContent(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	result := ScanFiles(context.Background(), nil, program, []ScanCandidate{{
		ReadPath: "main.go", Path: "main.go", Content: []byte("package p\nvar x = 123456789\n"),
	}}, ScanOptions{EvaluateOptions: EvaluateOptions{MaxSourceBytes: 16}})
	if findings := result.Findings(); len(findings) != 0 || !batchHasDiagnostic(result.Diagnostics(), "LIMIT_SOURCE_BYTES") {
		t.Fatalf("findings=%v diagnostics=%v", findings, result.Diagnostics())
	}
}
