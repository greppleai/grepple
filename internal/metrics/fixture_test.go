package metrics

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestComprehensiveSanitizedFixture(t *testing.T) {
	runs, err := AnalyzeFile(filepath.Join("testdata", "comprehensive-v3.jsonl"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	run := runs[0]
	assertFixtureIdentity(t, run)
	assertFixtureAggregation(t, run)
	assertFixtureDerivedMetrics(t, run)
}

func assertFixtureIdentity(t *testing.T, run Run) {
	t.Helper()
	if run.RunID != "fixture-run" || run.TaskID != "fixture-task" || !run.Complete {
		t.Fatalf("run identity = %#v", run)
	}
	if run.Provider != "fixture" || run.Model != "fixture-model" || run.ThinkingLevel != "high" {
		t.Fatalf("model metadata = %q/%q %q", run.Provider, run.Model, run.ThinkingLevel)
	}
	if run.ObservedGreppleUse || run.AssignedCohort != "grepple" {
		t.Fatalf("assignment and observed use were conflated: %#v", run)
	}
}

func assertFixtureAggregation(t *testing.T, run Run) {
	t.Helper()
	if run.Usage.TotalTokens != 109 || run.Usage.CacheRead != 20 || run.Usage.CacheWrite != 2 {
		t.Fatalf("usage = %#v", run.Usage)
	}
	if run.Turns != 4 || run.Tools.Total != 8 || run.Tools.Successful != 7 || run.Tools.Failed != 1 {
		t.Fatalf("turn/tool totals = turns %d, tools %#v", run.Turns, run.Tools)
	}
	if run.Compactions != 1 || run.Retries != 1 || run.RepeatedCalls != 2 || run.RedundantReads != 1 {
		t.Fatalf("navigation/retry totals = %#v", run)
	}
}

func assertFixtureDerivedMetrics(t *testing.T, run Run) {
	t.Helper()
	if run.TestFixCycles != 1 || run.RevertProxies != 1 || run.FirstPassingTest == nil {
		t.Fatalf("rework metrics = %#v", run)
	}
	if run.Outcome.FirstEditSurvived == nil || *run.Outcome.FirstEditSurvived || run.Outcome.Status != "success" {
		t.Fatalf("outcome = %#v", run.Outcome)
	}
	if !slices.Contains(run.Missing, "ambiguous_shell_classification") {
		t.Fatalf("missingness = %v", run.Missing)
	}
}

func TestAnalyzeSupportsV2AndKeepsIncompleteRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.jsonl")
	input := strings.Join([]string{
		`{"type":"session","version":2,"id":"v2","timestamp":"2026-01-01T00:00:00Z","cwd":"/repo"}`,
		`{"type":"custom","id":"start-1","parentId":null,"timestamp":"2026-01-01T00:00:01Z","customType":"grepple-metrics-v1","data":{"schema":1,"event":"run_start","runId":"one"}}`,
		`{"type":"custom","id":"pause","parentId":"start-1","timestamp":"2026-01-01T00:00:02Z","customType":"grepple-metrics-v1","data":{"schema":1,"event":"run_pause","runId":"one"}}`,
		`{"type":"custom","id":"start-2","parentId":"pause","timestamp":"2026-01-01T00:00:03Z","customType":"grepple-metrics-v1","data":{"schema":1,"event":"run_start","runId":"two"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	runs, err := AnalyzeFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Complete || runs[1].Complete {
		t.Fatalf("incomplete runs = %#v", runs)
	}
	if runs[0].RunID != "one" || runs[1].RunID != "two" {
		t.Fatalf("run ordering = %#v", runs)
	}
	for _, run := range runs {
		if !slices.Contains(run.Missing, "run_end") {
			t.Fatalf("run %q missing reasons = %v", run.RunID, run.Missing)
		}
	}
}

func TestDiscoverSessionsIsSortedAndDeduplicated(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z.jsonl", "nested/a.jsonl"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := DiscoverSessions([]string{root, filepath.Join(root, "z.jsonl")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || !slices.IsSorted(paths) {
		t.Fatalf("paths = %v", paths)
	}
}
