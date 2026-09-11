package gritql

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestScanFilesProgramsParsesEachFileOnceAndPreservesProgramResults(t *testing.T) {
	x := compileFindingPattern(t, "`x`")
	y := compileFindingPattern(t, "`y`")
	files := fstest.MapFS{
		"repo/b.go": {Data: []byte("package p\nvar y = 2\n")},
		"repo/a.go": {Data: []byte("package p\nvar x = 1\n")},
	}
	candidates := []ScanCandidate{
		{ReadPath: "repo/b.go", Path: "repo/b.go"},
		{ReadPath: "repo/a.go", Path: "repo/a.go"},
	}
	options := ScanOptions{IncludeGlobs: []string{"repo/*.go"}}
	batch := ScanFilesPrograms(context.Background(), files, []ProgramScan{
		{Program: x, PatternID: "x-rule", Message: "x found"},
		{Program: y, PatternID: "y-rule", Message: "y found"},
	}, candidates, options)

	if stats := batch.Stats(); stats.FilesRead != 2 || stats.FilesParsed != 2 {
		t.Fatalf("stats=%+v", stats)
	}
	results := batch.Programs()
	if len(results) != 2 {
		t.Fatalf("results=%d", len(results))
	}
	assertBatchFinding(t, results[0], "x-rule", "repo/a.go")
	assertBatchFinding(t, results[1], "y-rule", "repo/b.go")
}

func TestScanFilesProgramsCancellationInvalidatesEveryProgram(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	batch := ScanFilesPrograms(ctx, fstest.MapFS{"a.go": {Data: []byte("package p\nvar x = 1\n")}}, []ProgramScan{
		{Program: program, PatternID: "one"},
		{Program: program, PatternID: "two"},
	}, []ScanCandidate{{ReadPath: "a.go", Path: "a.go"}}, ScanOptions{})

	if stats := batch.Stats(); stats.FilesRead != 0 || stats.FilesParsed != 0 {
		t.Fatalf("stats=%+v", stats)
	}
	for _, result := range batch.Programs() {
		if len(result.Result.Findings()) != 0 || !batchHasDiagnostic(result.Result.Diagnostics(), "EVALUATION_CANCELLED") {
			t.Fatalf("result %q findings=%v diagnostics=%v", result.PatternID, result.Result.Findings(), result.Result.Diagnostics())
		}
	}
}

func assertBatchFinding(t *testing.T, result ProgramScanResult, patternID, path string) {
	t.Helper()
	findings := result.Result.Findings()
	if result.PatternID != patternID || len(findings) != 1 || findings[0].PatternID() != patternID || findings[0].Path() != path {
		t.Fatalf("result=%q findings=%v", result.PatternID, findings)
	}
	if diagnostics := result.Result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}

func batchHasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code() == code {
			return true
		}
	}
	return false
}
