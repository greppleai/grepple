package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

func TestMetricsReportAndCompareAreDeterministic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agents.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendCLITestEvent(t, path, "pi-run", "p-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{Agent: "pi"})
	appendCLITestEvent(t, path, "pi-run", "p-end", start.Add(2*time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})
	appendCLITestEvent(t, path, "claude-run", "c-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{Agent: "claude-code"})
	appendCLITestEvent(t, path, "claude-run", "c-end", start.Add(time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})

	before := metricReportOutputs(t, path)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_METRICS_DIR", filepath.Join(t.TempDir(), "ignored"))
	t.Chdir(t.TempDir())
	after := metricReportOutputs(t, path)
	for format, output := range before {
		if output != after[format] {
			t.Fatalf("nondeterministic %s report:\n%s\n%s", format, output, after[format])
		}
	}
	if !strings.Contains(before["json"], `"generatedAt": "2026-09-16T08:00:02Z"`) {
		t.Fatalf("JSON report does not derive time from evidence: %s", before["json"])
	}

	compareArgs := []string{"compare", "--input", path, "--baseline", "pi", "--target", "claude-code", "--format", "json"}
	firstComparison := runMetricsOutput(t, compareArgs)
	secondComparison := runMetricsOutput(t, compareArgs)
	if firstComparison != secondComparison || !strings.Contains(firstComparison, `"name": "pi"`) || !strings.Contains(firstComparison, `"name": "claude-code"`) {
		t.Fatalf("nondeterministic comparison:\n%s\n%s", firstComparison, secondComparison)
	}
}

func metricReportOutputs(t *testing.T, path string) map[string]string {
	t.Helper()
	formats := []string{"text", "json", "csv"}
	outputs := make(map[string]string, len(formats))
	for _, format := range formats {
		outputs[format] = runMetricsOutput(t, []string{"report", "--input", path, "--format", format})
	}
	return outputs
}

func runMetricsOutput(t *testing.T, args []string) string {
	t.Helper()
	return captureStdout(t, func() {
		if err := runMetrics(args); err != nil {
			t.Fatal(err)
		}
	})
}

func appendCLITestEvent(t *testing.T, path, runID, eventID string, at time.Time, eventType string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	event := agentmetrics.JournalEvent{Schema: agentmetrics.JournalSchema, EventID: eventID, Time: at, RunID: runID, Event: eventType, Data: data}
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
