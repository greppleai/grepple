package gritql

import (
	"context"
	"encoding/json"
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func TestScanFilesSkipsBinaryAndRejectsOversizedAndMalformedSources(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	files := fstest.MapFS{
		"binary.go": {Data: []byte("package p\x00var x = 1")},
		"huge.go":   {Data: []byte("package p\nvar x = 123456789\n")},
		"bad.go":    {Data: []byte("package p\nvar x =\n")},
		"good.go":   {Data: []byte("package p\nvar x = 1\n")},
	}
	result := ScanFiles(context.Background(), files, program, scanCandidates("binary.go", "huge.go", "bad.go", "good.go"), ScanOptions{
		PatternID: "rule", Workers: 3, EvaluateOptions: EvaluateOptions{MaxSourceBytes: 24},
	})
	if findings := result.Findings(); len(findings) != 1 || findings[0].Path() != "good.go" {
		t.Fatalf("findings=%v", findings)
	}
	codes := diagnosticCodes(result.Diagnostics())
	if got, want := mustJSON(t, codes), `["SOURCE_PARSE","LIMIT_SOURCE_BYTES"]`; got != want {
		t.Fatalf("diagnostic codes=%s want=%s", got, want)
	}
	if result.Stats().SkippedBinary != 1 {
		t.Fatalf("stats=%+v", result.Stats())
	}
}

func TestScanFilesReportsFileAndTotalByteTruncationSeparately(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	files := fstest.MapFS{
		"a.go": {Data: []byte("package p\nvar x=1\n")},
		"b.go": {Data: []byte("package p\nvar x=2\n")},
		"c.go": {Data: []byte("package p\nvar x=3\n")},
	}
	fileLimited := ScanFiles(context.Background(), files, program, scanCandidates("c.go", "b.go", "a.go"), ScanOptions{PatternID: "r", MaxFiles: 2})
	if got := fileLimited.Truncations(); len(got) != 1 || got[0].Reason != "max_files" || got[0].Skipped != 1 {
		t.Fatalf("file truncations=%+v", got)
	}
	if findings := fileLimited.Findings(); len(findings) != 2 || findings[0].Path() != "a.go" || findings[1].Path() != "b.go" {
		t.Fatalf("file-limited findings=%v", findings)
	}

	byteLimited := ScanFiles(context.Background(), files, program, scanCandidates("c.go", "b.go", "a.go"), ScanOptions{PatternID: "r", Workers: 3, MaxTotalBytes: 25})
	if got := byteLimited.Truncations(); len(got) != 1 || got[0].Reason != "max_total_bytes" || got[0].Skipped != 2 {
		t.Fatalf("byte truncations=%+v", got)
	}
	if findings := byteLimited.Findings(); len(findings) != 1 || findings[0].Path() != "a.go" {
		t.Fatalf("byte-limited findings=%v", findings)
	}
	if diagnostics := byteLimited.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("truncation leaked into diagnostics=%v", diagnostics)
	}
}

func TestScanFilesCanonicalDeduplicatesCandidatePaths(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	files := fstest.MapFS{
		"left.go":  {Data: []byte("package p\nvar x=1\n")},
		"right.go": {Data: []byte("package p\nvar x=2\n")},
	}
	result := ScanFiles(context.Background(), files, program, []ScanCandidate{
		{ReadPath: "right.go", Path: "same.go"},
		{ReadPath: "left.go", Path: "same.go"},
	}, ScanOptions{PatternID: "r", Workers: 2})
	findings := result.Findings()
	if len(findings) != 1 || findings[0].Path() != "same.go" || findings[0].Text() != "x" {
		t.Fatalf("findings=%v", findings)
	}
	if stats := result.Stats(); stats.Candidates != 2 || stats.Eligible != 1 || stats.Evaluated != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func TestScanFilesStopsAtAccountedMemoryLimit(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	files := fstest.MapFS{"a.go": {Data: []byte("package p\nvar x=1\n")}, "b.go": {Data: []byte("package p\nvar x=2\n")}}
	result := ScanFiles(context.Background(), files, program, scanCandidates("a.go", "b.go"), ScanOptions{
		PatternID: "r", EvaluateOptions: EvaluateOptions{MaxMemoryBytes: 64},
	})
	if len(result.Findings()) != 0 {
		t.Fatalf("findings=%v", result.Findings())
	}
	diagnostics := result.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Code() != "LIMIT_MEMORY" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if path, ok := diagnostics[0].Path(); !ok || path != "a.go" {
		t.Fatalf("memory diagnostic path=%q present=%v", path, ok)
	}
}
func TestScanFilesRejectsInvalidGlobConfiguration(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	result := ScanFiles(context.Background(), fstest.MapFS{"a.go": {Data: []byte("package p\nvar x=1\n")}}, program, scanCandidates("a.go"), ScanOptions{IncludeGlobs: []string{"["}})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "INTERNAL_ERROR" {
		t.Fatalf("invalid-glob result=%v/%v", result.Findings(), result.Diagnostics())
	}
}

func TestScanResultJSONUsesNonNullCollections(t *testing.T) {
	program := compileFindingPattern(t, "`missing`")
	result := ScanFiles(context.Background(), fstest.MapFS{}, program, nil, ScanOptions{})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Findings    []json.RawMessage `json:"findings"`
		Diagnostics []json.RawMessage `json:"diagnostics"`
		Truncations []json.RawMessage `json:"truncations"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Findings == nil || decoded.Diagnostics == nil || decoded.Truncations == nil {
		t.Fatalf("nullable collections: %s", encoded)
	}
}

func scanCandidates(paths ...string) []ScanCandidate {
	out := make([]ScanCandidate, len(paths))
	for index, path := range paths {
		out[index] = ScanCandidate{ReadPath: path, Path: path}
	}
	return out
}

func diagnosticCodes(diagnostics []Diagnostic) []string {
	out := make([]string, len(diagnostics))
	for index, diagnostic := range diagnostics {
		out[index] = diagnostic.Code()
	}
	return out
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestScanFilesBoundsConcurrencyAndCancelsWithoutPartialFindings(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	filesystem := &delayedFS{MapFS: fstest.MapFS{
		"a.go": {Data: []byte("package p\nvar x=1\n")},
		"b.go": {Data: []byte("package p\nvar x=2\n")},
		"c.go": {Data: []byte("package p\nvar x=3\n")},
	}, delay: 100 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan ScanResult, 1)
	go func() {
		done <- ScanFiles(ctx, filesystem, program, scanCandidates("a.go", "b.go", "c.go"), ScanOptions{PatternID: "r", Workers: 2})
	}()
	waitForConcurrentReads(t, &filesystem.maximum, 1)
	started := time.Now()
	cancel()
	result := <-done
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("cancellation waited for filesystem reads")
	}
	if filesystem.maximum.Load() > 2 {
		t.Fatalf("maximum concurrent reads=%d", filesystem.maximum.Load())
	}
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "EVALUATION_CANCELLED" {
		t.Fatalf("cancelled result=%v/%v", result.Findings(), result.Diagnostics())
	}
}

func TestScanFilesCountsAcquisitionAgainstFileDeadline(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	filesystem := &delayedFS{MapFS: fstest.MapFS{
		"a.go": {Data: []byte("package p\nvar x=1\n")},
	}, delay: 20 * time.Millisecond}
	result := ScanFiles(context.Background(), filesystem, program, scanCandidates("a.go"), ScanOptions{
		PatternID: "r", Workers: 1, EvaluateOptions: EvaluateOptions{MaxElapsed: 5 * time.Millisecond},
	})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "LIMIT_TIME_FILE" {
		t.Fatalf("deadline result=%v/%v", result.Findings(), result.Diagnostics())
	}
}

type delayedFS struct {
	fstest.MapFS
	delay   time.Duration
	active  atomic.Int32
	maximum atomic.Int32
}

func (d *delayedFS) Open(name string) (fs.File, error) {
	file, err := d.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	return &delayedFile{File: file, owner: d}, nil
}

type delayedFile struct {
	fs.File
	owner *delayedFS
}

func (f *delayedFile) Read(buffer []byte) (int, error) {
	active := f.owner.active.Add(1)
	for {
		maximum := f.owner.maximum.Load()
		if active <= maximum || f.owner.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	time.Sleep(f.owner.delay)
	count, err := f.File.Read(buffer)
	f.owner.active.Add(-1)
	return count, err
}

func waitForConcurrentReads(t *testing.T, maximum *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for maximum.Load() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if maximum.Load() < want {
		t.Fatalf("concurrent reads=%d, want %d", maximum.Load(), want)
	}
}
