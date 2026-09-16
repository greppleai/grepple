package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

func TestMetricsReportAndCompareAreDeterministic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cohorts.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendCLITestEvent(t, path, "control-run", "c-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{TaskID: "task", AssignedCohort: "control"})
	appendCLITestEvent(t, path, "control-run", "c-end", start.Add(2*time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})
	appendCLITestEvent(t, path, "grepple-run", "g-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{TaskID: "task", AssignedCohort: "grepple"})
	appendCLITestEvent(t, path, "grepple-run", "g-end", start.Add(time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})

	reportArgs := []string{"report", "--input", path, "--format", "json"}
	firstReport := captureStdout(t, func() {
		if err := runMetrics(reportArgs); err != nil {
			t.Fatal(err)
		}
	})
	secondReport := captureStdout(t, func() {
		if err := runMetrics(reportArgs); err != nil {
			t.Fatal(err)
		}
	})
	if firstReport != secondReport || !strings.Contains(firstReport, `"generatedAt": "2026-09-16T08:00:02Z"`) {
		t.Fatalf("nondeterministic report:\n%s\n%s", firstReport, secondReport)
	}

	compareArgs := []string{"compare", "--input", path, "--baseline", "control", "--target", "grepple", "--format", "json"}
	firstComparison := captureStdout(t, func() {
		if err := runMetrics(compareArgs); err != nil {
			t.Fatal(err)
		}
	})
	secondComparison := captureStdout(t, func() {
		if err := runMetrics(compareArgs); err != nil {
			t.Fatal(err)
		}
	})
	if firstComparison != secondComparison || !strings.Contains(firstComparison, `"name": "control"`) || !strings.Contains(firstComparison, `"name": "grepple"`) {
		t.Fatalf("nondeterministic comparison:\n%s\n%s", firstComparison, secondComparison)
	}
}

func appendCLITestEvent(t *testing.T, path, runID, eventID string, at time.Time, eventType string, value any) {
	t.Helper()
	data, err := agentmetrics.MarshalJournalData(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := agentmetrics.AppendJournalEvent(path, agentmetrics.JournalEvent{Schema: agentmetrics.JournalSchema, EventID: eventID, Time: at, RunID: runID, Event: eventType, Data: data}); err != nil {
		t.Fatal(err)
	}
}
