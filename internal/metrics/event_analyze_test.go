package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestAnalyzeFileBuildsRunFromNormalizedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendTestEvent(t, path, "run-1", "01", start, EventRunStart, RunStartData{Agent: "pi"})
	appendTestEvent(t, path, "run-1", "02", start.Add(time.Second), EventAssistant, AssistantData{Turn: 1, Usage: Usage{Input: 10, Output: 5, TotalTokens: 15, Cost: 0.02}})
	appendTestEvent(t, path, "run-1", "03", start.Add(2*time.Second), EventCommand, CommandData{Name: "architecture", Success: true, DurationMS: 12, GreppleMode: "architecture"})
	appendTestEvent(t, path, "run-1", "04", start.Add(3*time.Second), EventToolCall, ToolCallData{CallID: "read-1", Tool: "read", Category: "read", ArgumentShape: "resource-range", ResourceID: "resource-a", Turn: 1})
	appendTestEvent(t, path, "run-1", "05", start.Add(4*time.Second), EventToolResult, ToolResultData{CallID: "read-1", Success: true, Bytes: 80, Lines: 4})
	appendTestEvent(t, path, "run-1", "06", start.Add(5*time.Second), EventToolCall, ToolCallData{CallID: "edit-1", Tool: "edit", Category: "mutation", ArgumentShape: "resource", ResourceID: "resource-a", Turn: 1})
	added, removed := 3, 1
	appendTestEvent(t, path, "run-1", "07", start.Add(6*time.Second), EventToolResult, ToolResultData{CallID: "edit-1", Success: true, AddedLines: &added, RemovedLines: &removed, BeforeFingerprint: "before", AfterFingerprint: "after"})
	appendTestEvent(t, path, "run-1", "08", start.Add(7*time.Second), EventToolCall, ToolCallData{CallID: "test-1", Tool: "test", Category: "test", ArgumentShape: "test", Turn: 1})
	appendTestEvent(t, path, "run-1", "09", start.Add(8*time.Second), EventToolResult, ToolResultData{CallID: "test-1", Success: true, TestOutcome: "pass"})
	appendTestEvent(t, path, "run-1", "10", start.Add(9*time.Second), EventCompaction, struct{}{})
	score := 4.5
	appendTestEvent(t, path, "run-1", "11", start.Add(10*time.Second), EventRunEnd, RunEndData{Outcome: "success", EvaluatorScore: &score})

	runs, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	run := runs[0]
	if run.RunID != "run-1" || run.Agent != "pi" || !run.Complete || run.Outcome.Status != "success" {
		t.Fatalf("run identity/outcome = %#v", run)
	}
	if run.Turns != 1 || run.Usage.TotalTokens != 15 || run.Tools.Total != 4 || run.Tools.Grepple != 1 || run.Tools.Read != 1 || run.Tools.Mutation != 1 || run.Tools.Test != 1 {
		t.Fatalf("usage/tools = turns %d usage %#v tools %#v", run.Turns, run.Usage, run.Tools)
	}
	if run.ToolResultBytes != 80 || run.ToolResultLines != 4 || run.DistinctInspectedFiles != 1 || run.DistinctEditedFiles != 1 || run.AddedLines != 3 || run.RemovedLines != 1 {
		t.Fatalf("derived metrics = %#v", run)
	}
	if run.FirstEvidence == nil || run.FirstAttemptedMutation == nil || run.FirstSuccessfulMutation == nil || run.FirstPassingTest == nil || run.Completion == nil {
		t.Fatalf("milestones = %#v", run)
	}
	if !run.ObservedGreppleUse || run.GreppleModes["architecture"] != 1 || run.Compactions != 1 {
		t.Fatalf("Grepple/compaction metrics = %#v", run)
	}
}

func TestAnalyzeFileMarksUnavailableEvidenceMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendTestEvent(t, path, "run-1", "01", start, EventRunStart, RunStartData{Agent: "pi"})
	appendTestEvent(t, path, "run-1", "02", start.Add(time.Second), EventCommand, CommandData{Name: "search", Success: true, DurationMS: 10, GreppleMode: "search"})
	runs, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	missing := runs[0].Missing
	for _, want := range []string{"agent_turns", "assistant_usage", "first_attempted_mutation", "first_passing_test", "first_successful_mutation", "run_end", "semantic_relevance", "task_outcome", "tool_result_volume"} {
		if !slices.Contains(missing, want) {
			t.Errorf("missing evidence lacks %q: %v", want, missing)
		}
	}
	if runs[0].Usage.TotalTokens != 0 || runs[0].Complete {
		t.Fatalf("unavailable evidence became observed zero: %#v", runs[0])
	}
}

func TestAnalyzeGreppleJournalFixtureCoversSupportedEvidence(t *testing.T) {
	runs, err := AnalyzeFile(filepath.Join("testdata", "comprehensive-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %#v", runs)
	}
	target := runForAgent(runs, "pi")
	assertComprehensiveTarget(t, target)
	report := BuildReport(runs)
	if len(report.Groups) != 2 || !report.Generated.Equal(time.Date(2026, 1, 1, 0, 1, 20, 0, time.UTC)) {
		t.Fatalf("report = %#v", report)
	}
	comparison, err := BuildComparison(runs, "claude-code", "pi")
	if err != nil || comparison.Baseline.Name != "claude-code" || comparison.Target.Name != "pi" {
		t.Fatalf("comparison = %#v, err = %v", comparison, err)
	}
}

func runForAgent(runs []Run, agent string) Run {
	for _, run := range runs {
		if run.Agent == agent {
			return run
		}
	}
	return Run{}
}

func assertComprehensiveTarget(t *testing.T, target Run) {
	t.Helper()
	if target.RunID != "fixture-target" || !target.Complete || target.Outcome.Status != "success" {
		t.Fatalf("target identity/outcome = %#v", target)
	}
	if target.Usage.TotalTokens != 125 || target.Tools.Navigation != 2 || target.Tools.Read != 2 || target.Tools.Mutation != 2 || target.Tools.Test != 2 || target.Tools.Grepple != 2 {
		t.Fatalf("target usage/tools = usage %#v tools %#v", target.Usage, target.Tools)
	}
	if target.AddedLines != 4 || target.RemovedLines != 1 || target.RepeatedCalls == 0 || target.RedundantReads == 0 || target.SearchToRead == 0 || target.SearchToEdit == 0 || target.TestFixCycles == 0 {
		t.Fatalf("target derived metrics = %#v", target)
	}
	if target.FirstEvidence == nil || target.FirstAttemptedMutation == nil || target.FirstSuccessfulMutation == nil || target.FirstPassingTest == nil || target.Completion == nil {
		t.Fatalf("target milestones = %#v", target)
	}
	if !slices.Contains(target.Missing, "retries") || !slices.Contains(target.Missing, "semantic_relevance") {
		t.Fatalf("target missing evidence = %v", target.Missing)
	}
}

func TestAnalyzeFileRejectsInvalidLifecycleFromJSONL(t *testing.T) {
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		write func(*testing.T, string)
	}{
		{name: "before start", write: func(t *testing.T, path string) {
			appendTestEvent(t, path, "run-1", "command", start, EventCommand, CommandData{Name: "search"})
		}},
		{name: "duplicate start", write: func(t *testing.T, path string) {
			appendTestEvent(t, path, "run-1", "start-1", start, EventRunStart, RunStartData{Agent: "pi"})
			appendTestEvent(t, path, "run-1", "start-2", start.Add(time.Second), EventRunStart, RunStartData{Agent: "pi"})
		}},
		{name: "after end", write: func(t *testing.T, path string) {
			appendTestEvent(t, path, "run-1", "start", start, EventRunStart, RunStartData{Agent: "pi"})
			appendTestEvent(t, path, "run-1", "end", start.Add(time.Second), EventRunEnd, RunEndData{Outcome: "success"})
			appendTestEvent(t, path, "run-1", "command", start.Add(2*time.Second), EventCommand, CommandData{Name: "search"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "run.jsonl")
			test.write(t, path)
			if _, err := AnalyzeFile(path); err == nil {
				t.Fatal("expected lifecycle error")
			}
		})
	}
}

func TestDecodeEventDataRejectsUnsafeOrInvalidNormalizedFields(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      string
	}{
		{name: "content-bearing path", eventType: EventToolCall, data: `{"callId":"call-1","tool":"read","category":"read","argumentShape":"resource","resourceId":"secret/path.go","turn":1}`},
		{name: "unknown category", eventType: EventToolCall, data: `{"callId":"call-1","tool":"read","category":"mystery","argumentShape":"resource","turn":1}`},
		{name: "negative range", eventType: EventToolCall, data: `{"callId":"call-1","tool":"read","category":"read","argumentShape":"resource","offset":-1,"turn":1}`},
		{name: "negative churn", eventType: EventToolResult, data: `{"callId":"call-1","success":true,"bytes":0,"lines":0,"addedLines":-1}`},
		{name: "negative evaluation", eventType: EventRunEnd, data: `{"outcome":"success","humanInterventions":-1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEventData(test.eventType, []byte(test.data)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func appendTestEvent(t *testing.T, path, runID, eventID string, at time.Time, eventType string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	event := JournalEvent{Schema: JournalSchema, EventID: fmt.Sprintf("event-%s", eventID), Time: at, RunID: runID, Event: eventType, Data: data}
	line, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
