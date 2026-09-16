package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

func TestMetricsLifecycleWritesGreppleJournal(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(metricsDirectoryEnv, directory)

	startOutput := captureStdout(t, func() {
		if err := runMetrics([]string{"start", "--run", "run-1", "--event-id", "start-1", "--at", "2026-09-16T08:00:00Z", "--task", "task-1", "--cohort", "grepple", "--repository", "repo", "--revision", "abc"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(startOutput, "run-1") {
		t.Fatalf("start output = %q", startOutput)
	}

	statusOutput := captureStdout(t, func() {
		if err := runMetrics([]string{"status"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(statusOutput, "run-1") || !strings.Contains(statusOutput, "task-1") {
		t.Fatalf("status output = %q", statusOutput)
	}

	assistant := `{"turn":1,"provider":"provider","model":"model","thinkingLevel":"high","usage":{"input":10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":15,"cost":0.02}}`
	if err := runMetrics([]string{"record", "--event", "assistant", "--event-id", "assistant-1", "--at", "2026-09-16T08:00:01Z", "--data", assistant}); err != nil {
		t.Fatal(err)
	}
	if err := runMetrics([]string{"end", "--event-id", "end-1", "--at", "2026-09-16T08:01:00Z", "--outcome", "success", "--score", "4.5", "--human-interventions", "0", "--regressions", "0", "--first-edit-survived", "true", "--rubric", "unit"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "runs", "run-1.jsonl")
	events, err := agentmetrics.ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Event != agentmetrics.EventRunStart || events[1].Event != agentmetrics.EventAssistant || events[2].Event != agentmetrics.EventRunEnd {
		t.Fatalf("events = %#v", events)
	}
	decoded, err := agentmetrics.DecodeEventData(events[2].Event, events[2].Data)
	if err != nil {
		t.Fatal(err)
	}
	end := decoded.(*agentmetrics.RunEndData)
	if end.Outcome != "success" || end.EvaluatorScore == nil || *end.EvaluatorScore != 4.5 {
		t.Fatalf("end data = %#v", end)
	}
	if _, err := os.Stat(metricsActiveStatePath(directory)); !os.IsNotExist(err) {
		t.Fatalf("active state still exists: %v", err)
	}
}

func TestConcurrentMetricsStartsClaimOneActiveState(t *testing.T) {
	directory := t.TempDir()
	state := metricsActiveState{
		Schema: activeStateSchema, RunID: "run-1", TaskID: "task-1", AssignedCohort: "control",
		Journal: filepath.Join(directory, "runs", "run-1.jsonl"), RepositoryRootHash: metricsRepositoryHash(metricsRepositoryRoot()),
	}
	const attempts = 8
	errors := make(chan error, attempts)
	var group sync.WaitGroup
	for index := 0; index < attempts; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- writeMetricsActiveState(directory, state)
		}()
	}
	group.Wait()
	close(errors)
	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful active-state claims = %d, want 1", successes)
	}
}

func TestMetricsRecordRejectsContentBearingUnknownFields(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(metricsDirectoryEnv, directory)
	if err := runMetrics([]string{"start", "--run", "run-1", "--event-id", "start-1", "--at", "2026-09-16T08:00:00Z", "--task", "task-1", "--cohort", "control"}); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"turn": 1, "prompt": "secret"})
	err := runMetrics([]string{"record", "--event", "assistant", "--event-id", "assistant-1", "--at", "2026-09-16T08:00:01Z", "--data", string(data)})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
	pathData, _ := json.Marshal(map[string]any{"callId": "call-1", "tool": "read", "category": "read", "argumentShape": "resource", "path": "secret/name.go", "turn": 1})
	err = runMetrics([]string{"record", "--event", "tool_call", "--event-id", "call-1", "--at", "2026-09-16T08:00:02Z", "--data", string(pathData)})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("path error = %v", err)
	}
}
