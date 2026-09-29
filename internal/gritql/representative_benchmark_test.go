package gritql

import (
	"context"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

type structuralBenchmarkProfile struct {
	name  string
	files int
}

type structuralBenchmarkLanguage struct {
	language, extension, source, comment string
}

var structuralBenchmarkProfiles = []structuralBenchmarkProfile{
	{name: "medium", files: 500},
	{name: "large", files: 5_000},
}

var structuralBenchmarkLanguages = []structuralBenchmarkLanguage{
	{language: "c", extension: "c", source: "void run(void) { target(one, two); }\n", comment: "// padding\n"},
	{language: "cpp", extension: "cpp", source: "void run() { target(one, two); }\n", comment: "// padding\n"},
	{language: "csharp", extension: "cs", source: "class App { void Run() { Target(one, two); } }\n", comment: "// padding\n"},
	{language: "dart", extension: "dart", source: "void run() { target(one, two); }\n", comment: "// padding\n"},
	{language: "go", extension: "go", source: "package sample\nfunc run() { target(one, two) }\n", comment: "// padding\n"},
	{language: "javascript", extension: "js", source: "const result = target(one, two);\n", comment: "// padding\n"},
	{language: "java", extension: "java", source: "class App { void run() { target(one, two); } }\n", comment: "// padding\n"},
	{language: "kotlin", extension: "kt", source: "fun run() { target(one, two) }\n", comment: "// padding\n"},
	{language: "php", extension: "php", source: "<?php function run() { target($one, $two); }\n", comment: "// padding\n"},
	{language: "python", extension: "py", source: "result = target(one, two)\n", comment: "# padding\n"},
	{language: "rust", extension: "rs", source: "fn run() { target(one, two); }\n", comment: "// padding\n"},
	{language: "shell", extension: "sh", source: "run() { target one two; }\n", comment: "# padding\n"},
	{language: "typescript", extension: "ts", source: "const result = target(one, two);\n", comment: "// padding\n"},
	{language: "swift", extension: "swift", source: "func run() { target(one, two) }\n", comment: "// padding\n"},
	{language: "tsx", extension: "tsx", source: "const result = target(one, two);\n", comment: "// padding\n"},
}

func TestRepresentativeStructuralBenchmarkLanguagesCoverRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, language := range SupportedLanguages() {
		registered[language.ID] = true
	}
	covered := map[string]bool{}
	for _, language := range structuralBenchmarkLanguages {
		if covered[language.language] {
			t.Errorf("benchmark language %q is duplicated", language.language)
		}
		covered[language.language] = true
		if !registered[language.language] {
			t.Errorf("benchmark language %q is not registered", language.language)
		}
	}
	for language := range registered {
		if !covered[language] {
			t.Errorf("registered language %q is absent from representative benchmarks", language)
		}
	}
}
func BenchmarkRepresentativeStructuralScans(b *testing.B) {
	for _, profile := range structuralBenchmarkProfiles {
		profile := profile
		b.Run(profile.name, func(b *testing.B) {
			filesystem, candidates, sourceBytes := representativeStructuralCorpus(profile.files)
			b.Run("cold-compile-and-scan", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(sourceBytes)
				for b.Loop() {
					programs := benchmarkStructuralPrograms(b)
					assertRepresentativeScan(b, ScanFilesPrograms(context.Background(), filesystem, programs, candidates, ScanOptions{Workers: runtime.GOMAXPROCS(0)}), profile.files)
				}
			})
			programs := benchmarkStructuralPrograms(b)
			b.Run("warm-precompiled", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(sourceBytes)
				for b.Loop() {
					assertRepresentativeScan(b, ScanFilesPrograms(context.Background(), filesystem, programs, candidates, ScanOptions{Workers: runtime.GOMAXPROCS(0)}), profile.files)
				}
			})
		})
	}
}

func BenchmarkRepresentativeStructuralScanPeakHeap(b *testing.B) {
	for _, profile := range structuralBenchmarkProfiles {
		profile := profile
		b.Run(profile.name, func(b *testing.B) {
			filesystem, candidates, sourceBytes := representativeStructuralCorpus(profile.files)
			programs := benchmarkStructuralPrograms(b)
			var totalPeak uint64
			b.ReportAllocs()
			b.SetBytes(sourceBytes)
			for b.Loop() {
				runtime.GC()
				stop := make(chan struct{})
				peak := make(chan uint64, 1)
				go sampleStructuralHeap(stop, peak)
				result := ScanFilesPrograms(context.Background(), filesystem, programs, candidates, ScanOptions{Workers: runtime.GOMAXPROCS(0)})
				close(stop)
				totalPeak += <-peak
				assertRepresentativeScan(b, result, profile.files)
			}
			b.ReportMetric(float64(totalPeak)/float64(b.N), "peak-heap-bytes/op")
		})
	}
}

func BenchmarkRepresentativeStructuralScanCancellationLatency(b *testing.B) {
	filesystem, candidates, sourceBytes := representativeStructuralCorpus(structuralBenchmarkProfiles[1].files)
	programs := benchmarkStructuralPrograms(b)
	var totalLatency time.Duration
	b.ReportAllocs()
	b.SetBytes(sourceBytes)
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		signaled := &structuralBenchmarkSignalFS{FS: filesystem, started: started}
		cancelledAt := make(chan time.Time, 1)
		go func() {
			<-started
			cancelledAt <- time.Now()
			cancel()
		}()
		result := ScanFilesPrograms(ctx, signaled, programs, candidates, ScanOptions{Workers: runtime.GOMAXPROCS(0)})
		totalLatency += time.Since(<-cancelledAt)
		if findings, diagnostics := representativeScanCounts(result); findings != 0 || !batchHasDiagnostic(diagnostics, "EVALUATION_CANCELLED") {
			b.Fatalf("cancelled scan was not transactional: findings=%d diagnostics=%v", findings, diagnostics)
		}
	}
	b.ReportMetric(float64(totalLatency.Nanoseconds())/float64(b.N), "cancel-ns/op")
}

func representativeStructuralCorpus(count int) (fstest.MapFS, []ScanCandidate, int64) {
	filesystem := make(fstest.MapFS, count)
	candidates := make([]ScanCandidate, count)
	var sourceBytes int64
	for index := range candidates {
		language := structuralBenchmarkLanguages[index%len(structuralBenchmarkLanguages)]
		paddingLines := []int{0, 8, 64, 256}[index%4]
		content := []byte(language.source + strings.Repeat(language.comment, paddingLines))
		path := fmt.Sprintf("repo/module-%02d/package-%03d/file-%06d.%s", index%24, index%240, index, language.extension)
		filesystem[path] = &fstest.MapFile{Data: content, Mode: 0o644}
		candidates[index] = ScanCandidate{ReadPath: path, Path: path, Language: language.language}
		sourceBytes += int64(len(content))
	}
	return filesystem, candidates, sourceBytes
}

func benchmarkStructuralPrograms(b *testing.B) []ProgramScan {
	b.Helper()
	programs := make([]ProgramScan, 0, len(structuralBenchmarkLanguages))
	for _, language := range structuralBenchmarkLanguages {
		callee := "target"
		separator := "("
		suffix := ")"
		if language.language == "csharp" {
			callee = "Target"
		}
		if language.language == "shell" {
			separator = " "
			suffix = ""
		}
		query := fmt.Sprintf("language %s\n`%s%s$args%s`", language.language, callee, separator, suffix)
		program, err := Compile([]byte(query), CompileOptions{})
		if err != nil {
			b.Fatalf("compile %s benchmark: %v", language.language, err)
		}
		programs = append(programs, ProgramScan{Program: program, PatternID: language.language})
	}
	return programs
}

func assertRepresentativeScan(b *testing.B, result BatchScanResult, wantFiles int) {
	b.Helper()
	findings, diagnostics := representativeScanCounts(result)
	if len(diagnostics) != 0 {
		b.Fatalf("representative scan diagnostics=%v", diagnostics)
	}
	if findings != wantFiles {
		b.Fatalf("representative scan findings=%d want %d stats=%+v", findings, wantFiles, result.Stats())
	}
}

func representativeScanCounts(result BatchScanResult) (int, []Diagnostic) {
	findings := 0
	var diagnostics []Diagnostic
	for _, program := range result.Programs() {
		findings += len(program.Result.Findings())
		diagnostics = append(diagnostics, program.Result.Diagnostics()...)
	}
	return findings, diagnostics
}

func sampleStructuralHeap(stop <-chan struct{}, result chan<- uint64) {
	ticker := time.NewTicker(100 * time.Microsecond)
	defer ticker.Stop()
	var peak uint64
	observe := func() {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if stats.HeapAlloc > peak {
			peak = stats.HeapAlloc
		}
	}
	observe()
	for {
		select {
		case <-ticker.C:
			observe()
		case <-stop:
			observe()
			result <- peak
			return
		}
	}
}

type structuralBenchmarkSignalFS struct {
	fs.FS
	started chan struct{}
	once    sync.Once
}

func (filesystem *structuralBenchmarkSignalFS) Open(name string) (fs.File, error) {
	filesystem.once.Do(func() { close(filesystem.started) })
	return filesystem.FS.Open(name)
}
