package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderedContextGuardOmitsRepeatedSourceBlocks(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	firstBlock := "source.go\n\n" + strings.Repeat("abc│12│func unchanged() {}\n", 8) + "\n"
	secondBlock := strings.Repeat("def│30│func changed() {}\n", 8)
	first := []byte(firstBlock + secondBlock)
	guarded, err := guardRenderedOutput(first)
	if err != nil || string(guarded) != string(first) {
		t.Fatalf("first rendering changed: %q err=%v", guarded, err)
	}
	modified := []byte(firstBlock + strings.ReplaceAll(secondBlock, "changed", "newValue"))
	guarded, err = guardRenderedOutput(modified)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guarded), "omitted 1 unchanged source block") || strings.Contains(string(guarded), "unchanged()") {
		t.Fatalf("repeated block was not omitted: %q", guarded)
	}
	if !strings.Contains(string(guarded), "newValue") {
		t.Fatalf("modified block was omitted: %q", guarded)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.ObservedCalls != 2 || stats.NewBlocks != 3 || stats.RemovedBlocks != 1 || stats.NetSavedBytes <= 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestRenderedContextInvalidationRotatesStatsAndRestoresOutput(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	content := []byte(strings.Repeat("abc│1│external dependency declaration\n", 8))
	if _, err := guardRenderedOutput(content); err != nil {
		t.Fatal(err)
	}
	if err := invalidateRenderedContext("compact"); err != nil {
		t.Fatal(err)
	}
	rotated := readRenderedContextStats(filepath.Join(directory, "stats-1.json"), 1)
	if rotated.Period != 1 || rotated.ResetReason != "compact" || rotated.ObservedCalls != 0 {
		t.Fatalf("stats did not rotate: %#v", rotated)
	}
	guarded, err := guardRenderedOutput(content)
	if err != nil || string(guarded) != string(content) {
		t.Fatalf("invalidated output remained suppressed: %q err=%v", guarded, err)
	}
	if stats := readRenderedContextStats(filepath.Join(directory, "stats-1.json"), 1); stats.ObservedCalls != 1 {
		t.Fatalf("new period not updated: %#v", stats)
	}
}

func TestRenderedContextGuardIgnoresNonSourceAndJSON(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	plain := []byte(strings.Repeat("ordinary command output\n", 20))
	guarded, err := guardRenderedOutput(plain)
	if err != nil || string(guarded) != string(plain) {
		t.Fatalf("non-source output changed: %q err=%v", guarded, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "cache.json")); !os.IsNotExist(err) {
		t.Fatalf("non-source output created cache: %v", err)
	}
	path := filepath.Join(t.TempDir(), "output.json")
	jsonOutput := []byte(`{"line":"abc│1│source"}`)
	if err := os.WriteFile(path, jsonOutput, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := guardRenderedOutputFile(path, true)
	if err != nil || string(result) != string(jsonOutput) {
		t.Fatalf("JSON output changed: %q err=%v", result, err)
	}
}

func TestSpilledSourceIsNotRecordedByContextGuard(t *testing.T) {
	guardDirectory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", guardDirectory)
	outputDirectory := t.TempDir()
	temporary, err := os.CreateTemp(outputDirectory, ".spill-*")
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("abc│1│large external declaration\n", 20)
	if _, err := temporary.WriteString(content); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := finishOutputSpill(temporary, temporary.Name(), outputDirectory, 1, []string{"--at", "external.go:1"}, &stdout, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "grepple output spilled") {
		t.Fatalf("missing spill descriptor: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(guardDirectory, "cache.json")); !os.IsNotExist(err) {
		t.Fatalf("spilled source was recorded: %v", err)
	}
}

func TestRunContextInvalidateValidation(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	if err := runContext([]string{"invalidate", "--reason", "compact"}); err != nil {
		t.Fatal(err)
	}
	if err := runContext([]string{"invalidate", "--unknown"}); err == nil {
		t.Fatal("unknown argument accepted")
	}
}
