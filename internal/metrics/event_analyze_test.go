package metrics

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestAnalyzeFileBuildsRunFromNormalizedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendTestEvent(t, path, "run-1", "01", start, EventRunStart, RunStartData{TaskID: "task-1", Repository: "repo", Revision: "abc", AssignedCohort: "grepple"})
	appendTestEvent(t, path, "run-1", "02", start.Add(time.Second), EventAssistant, AssistantData{Turn: 1, Provider: "provider", Model: "model", ThinkingLevel: "high", Usage: Usage{Input: 10, Output: 5, TotalTokens: 15, Cost: 0.02}})
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
	if run.RunID != "run-1" || run.TaskID != "task-1" || !run.Complete || run.Outcome.Status != "success" {
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
	appendTestEvent(t, path, "run-1", "01", start, EventRunStart, RunStartData{TaskID: "task-1", AssignedCohort: "control"})
	appendTestEvent(t, path, "run-1", "02", start.Add(time.Second), EventCommand, CommandData{Name: "search", Success: true, DurationMS: 10, GreppleMode: "search"})
	runs, err := AnalyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	missing := runs[0].Missing
	for _, want := range []string{"agent_turns", "first_attempted_mutation", "first_passing_test", "first_successful_mutation", "model_usage", "run_end", "semantic_relevance", "task_outcome", "tool_result_volume"} {
		if !slices.Contains(missing, want) {
			t.Errorf("missing evidence lacks %q: %v", want, missing)
		}
	}
	if runs[0].Usage.TotalTokens != 0 || runs[0].Complete {
		t.Fatalf("unavailable evidence became observed zero: %#v", runs[0])
	}
}

func TestAnalyzeGreppleJournalFixture(t *testing.T) {
	runs, err := AnalyzeFile(filepath.Join("testdata", "comprehensive-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].RunID != "fixture-run" || runs[0].Usage.TotalTokens != 125 || runs[0].Tools.Navigation != 1 || runs[0].Outcome.Status != "success" {
		t.Fatalf("fixture runs = %#v", runs)
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
	data, err := MarshalJournalData(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendJournalEvent(path, JournalEvent{Schema: JournalSchema, EventID: fmt.Sprintf("event-%s", eventID), Time: at, RunID: runID, Event: eventType, Data: data}); err != nil {
		t.Fatal(err)
	}
}
