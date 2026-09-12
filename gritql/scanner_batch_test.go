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
func TestScanFilesProgramsPrefiltersPerProgramBeforeSharedParse(t *testing.T) {
	target := compileFindingPattern(t, "`target($value)`")
	other := compileFindingPattern(t, "`other($value)`")
	batch := ScanFilesPrograms(context.Background(), fstest.MapFS{
		"main.go": {Data: []byte("package p\nvar _ = target(value)\n")},
	}, []ProgramScan{{Program: target, PatternID: "target"}, {Program: other, PatternID: "other"}}, []ScanCandidate{{ReadPath: "main.go", Path: "main.go"}}, ScanOptions{})
	if stats := batch.Stats(); stats.FilesRead != 1 || stats.FilesParsed != 1 {
		t.Fatalf("batch stats=%+v", stats)
	}
	results := batch.Programs()
	assertBatchFinding(t, results[0], "target", "main.go")
	if stats := results[1].Result.Stats(); stats.Evaluated != 0 || stats.SkippedAnchor != 1 {
		t.Fatalf("other stats=%+v", stats)
	}
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
func TestScanFilesProgramsGroupsMixedTargetLanguages(t *testing.T) {
	goProgram := compileFindingPattern(t, "`target($value)`")
	typeScriptProgram, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	javaScriptProgram, err := Compile([]byte("language javascript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pythonProgram, err := Compile([]byte("language python\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	batch := ScanFilesPrograms(context.Background(), fstest.MapFS{
		"main.go": {Data: []byte("package p\nvar _ = target(value)\n")},
		"app.ts":  {Data: []byte("target(value);\n")},
		"app.js":  {Data: []byte("target(value);\n")},
		"app.py":  {Data: []byte("target(value)\n")},
	}, []ProgramScan{{Program: goProgram, PatternID: "go"}, {Program: typeScriptProgram, PatternID: "typescript"}, {Program: javaScriptProgram, PatternID: "javascript"}, {Program: pythonProgram, PatternID: "python"}}, []ScanCandidate{
		{ReadPath: "main.go", Path: "main.go"}, {ReadPath: "app.ts", Path: "app.ts"}, {ReadPath: "app.js", Path: "app.js"}, {ReadPath: "app.py", Path: "app.py"},
	}, ScanOptions{})
	if stats := batch.Stats(); stats.FilesRead != 4 || stats.FilesParsed != 4 {
		t.Fatalf("stats=%+v", stats)
	}
	results := batch.Programs()
	if len(results) != 4 {
		t.Fatalf("results=%d", len(results))
	}
	assertBatchFinding(t, results[0], "go", "main.go")
	assertBatchFinding(t, results[1], "typescript", "app.ts")
	assertBatchFinding(t, results[2], "javascript", "app.js")
	assertBatchFinding(t, results[3], "python", "app.py")
}
func TestScanFilesProgramsReportsSharedByteLimitPerLanguage(t *testing.T) {
	goProgram := compileFindingPattern(t, "`target($value)`")
	typeScriptProgram, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	typeScriptSource := []byte("target(value);\n")
	batch := ScanFilesPrograms(context.Background(), fstest.MapFS{
		"app.ts":  {Data: typeScriptSource},
		"main.go": {Data: []byte("package p\nvar _ = target(one)\n")},
		"z.go":    {Data: []byte("package p\nvar _ = target(two)\n")},
	}, []ProgramScan{{Program: goProgram, PatternID: "go"}, {Program: typeScriptProgram, PatternID: "typescript"}}, []ScanCandidate{
		{ReadPath: "app.ts", Path: "app.ts"},
		{ReadPath: "main.go", Path: "main.go"},
		{ReadPath: "z.go", Path: "z.go"},
	}, ScanOptions{MaxTotalBytes: int64(len(typeScriptSource) + 1)})
	if stats := batch.Stats(); stats.FilesRead != 2 || stats.FilesParsed != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	results := batch.Programs()
	goTruncations := results[0].Result.Truncations()
	if len(goTruncations) != 1 || goTruncations[0].Reason != "max_total_bytes" || goTruncations[0].Skipped != 2 {
		t.Fatalf("Go truncations=%v", goTruncations)
	}
	if truncations := results[1].Result.Truncations(); len(truncations) != 0 {
		t.Fatalf("TypeScript truncations=%v", truncations)
	}
	assertBatchFinding(t, results[1], "typescript", "app.ts")
}
func TestScanFilesProgramsRejectsInvalidProgramWithoutReading(t *testing.T) {
	batch := ScanFilesPrograms(context.Background(), fstest.MapFS{
		"main.go": {Data: []byte("package p\nvar x = 1\n")},
	}, []ProgramScan{{PatternID: "invalid"}}, []ScanCandidate{{ReadPath: "main.go", Path: "main.go"}}, ScanOptions{})
	if stats := batch.Stats(); stats.FilesRead != 0 || stats.FilesParsed != 0 {
		t.Fatalf("stats=%+v", stats)
	}
	results := batch.Programs()
	if len(results) != 1 || !batchHasDiagnostic(results[0].Result.Diagnostics(), "INTERNAL_ERROR") {
		t.Fatalf("results=%v", results)
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
