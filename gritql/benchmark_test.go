package gritql

import (
	"context"
	"fmt"
	"testing"
	"testing/fstest"
)

func BenchmarkCompileStructuralQuery(b *testing.B) {
	query := []byte("language go\n`target($x)` where { $x <: `value($_)` }")
	b.ReportAllocs()
	b.SetBytes(int64(len(query)))
	for b.Loop() {
		if _, err := Compile(query, CompileOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluateFileStructuralQuery(b *testing.B) {
	program, err := Compile([]byte("language go\n`target($x)`"), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	source := []byte("package p\nfunc f() { target(value(1)); target(value(2)) }\n")
	input := FileInput{Path: "repo/main.go", Content: source, PatternID: "benchmark"}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for b.Loop() {
		result := EvaluateFile(context.Background(), program, input, EvaluateOptions{})
		if len(result.Findings()) != 2 {
			b.Fatalf("findings=%d", len(result.Findings()))
		}
	}
}
func BenchmarkEvaluateTypeScriptStructuralQuery(b *testing.B) {
	program, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	source := []byte("const first = target(value(1)); const second = target(value(2));\n")
	input := FileInput{Path: "repo/main.ts", Language: "typescript", Content: source, PatternID: "benchmark"}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for b.Loop() {
		result := EvaluateFile(context.Background(), program, input, EvaluateOptions{})
		if len(result.Findings()) != 2 {
			b.Fatalf("findings=%d", len(result.Findings()))
		}
	}
}

func BenchmarkScanFilesProgramsSharedParse(b *testing.B) {
	first, err := Compile([]byte("language go\n`target($x)`"), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	second, err := Compile([]byte("language go\n`value($x)`"), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	filesystem := fstest.MapFS{}
	candidates := make([]ScanCandidate, 100)
	for index := range candidates {
		path := fmt.Sprintf("repo/file%03d.go", index)
		filesystem[path] = &fstest.MapFile{Data: []byte("package p\nfunc f() { target(value(1)) }\n")}
		candidates[index] = ScanCandidate{ReadPath: path, Path: path}
	}
	programs := []ProgramScan{{Program: first, PatternID: "target"}, {Program: second, PatternID: "value"}}
	b.ReportAllocs()
	for b.Loop() {
		result := ScanFilesPrograms(context.Background(), filesystem, programs, candidates, ScanOptions{Workers: 1})
		if result.Stats().FilesParsed != len(candidates) {
			b.Fatalf("parsed=%d", result.Stats().FilesParsed)
		}
	}
}

func BenchmarkScanFilesAnchored(b *testing.B) {
	benchmarkScanFiles(b, "language go\n`target($x)`", true)
}

func BenchmarkScanFilesUnanchored(b *testing.B) {
	benchmarkScanFiles(b, "language go\n`$x`", false)
}

func benchmarkScanFiles(b *testing.B, query string, wantAnchor bool) {
	b.Helper()
	program, err := Compile([]byte(query), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	if (len(AnalyzeAnchors(program).RequiredLiterals()) > 0) != wantAnchor {
		b.Fatalf("anchor classification does not match benchmark: %q", query)
	}
	filesystem, candidates := benchmarkFileSet(100)
	b.ReportAllocs()
	for b.Loop() {
		result := ScanFiles(context.Background(), filesystem, program, candidates, ScanOptions{Workers: 1})
		if result.Stats().Evaluated != len(candidates) {
			b.Fatalf("evaluated=%d", result.Stats().Evaluated)
		}
	}
}

func BenchmarkScanFilesCancellationLatency(b *testing.B) {
	program, err := Compile([]byte("language go\n`target($x)`"), CompileOptions{})
	if err != nil {
		b.Fatal(err)
	}
	filesystem, candidates := benchmarkFileSet(100)
	b.ReportAllocs()
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := ScanFiles(ctx, filesystem, program, candidates, ScanOptions{Workers: 4})
		if len(result.Findings()) != 0 || !batchHasDiagnostic(result.Diagnostics(), "EVALUATION_CANCELLED") {
			b.Fatalf("cancelled scan was not transactional: %#v", result)
		}
	}
}

func benchmarkFileSet(count int) (fstest.MapFS, []ScanCandidate) {
	filesystem := fstest.MapFS{}
	candidates := make([]ScanCandidate, count)
	for index := range candidates {
		path := fmt.Sprintf("repo/file%03d.go", index)
		filesystem[path] = &fstest.MapFile{Data: []byte("package p\nfunc f() { target(value(1)) }\n")}
		candidates[index] = ScanCandidate{ReadPath: path, Path: path}
	}
	return filesystem, candidates
}
