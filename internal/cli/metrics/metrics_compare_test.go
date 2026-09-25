package metrics

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
	directory := t.TempDir()
	baselineJournal := filepath.Join(directory, "without-grepple.jsonl")
	targetJournal := filepath.Join(directory, "with-grepple.jsonl")
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	appendCLITestEvent(t, baselineJournal, "baseline-run", "b-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{Agent: "pi"})
	appendCLITestEvent(t, baselineJournal, "baseline-run", "b-end", start.Add(2*time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})
	appendCLITestEvent(t, targetJournal, "target-run", "t-start", start, agentmetrics.EventRunStart, agentmetrics.RunStartData{Agent: "pi"})
	appendCLITestEvent(t, targetJournal, "target-run", "t-command", start.Add(500*time.Millisecond), agentmetrics.EventCommand, agentmetrics.CommandData{Name: "search", Success: true, DurationMS: 10, GreppleMode: "search"})
	appendCLITestEvent(t, targetJournal, "target-run", "t-end", start.Add(time.Second), agentmetrics.EventRunEnd, agentmetrics.RunEndData{Outcome: "success"})

	before := metricReportOutputs(t, baselineJournal)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_METRICS_DIR", filepath.Join(t.TempDir(), "ignored"))
	t.Chdir(t.TempDir())
	after := metricReportOutputs(t, baselineJournal)
	for format, output := range before {
		if output != after[format] {
			t.Fatalf("nondeterministic %s report:\n%s\n%s", format, output, after[format])
		}
	}
	if !strings.Contains(before["json"], `"generatedAt": "2026-09-16T08:00:02Z"`) {
		t.Fatalf("JSON report does not derive time from evidence: %s", before["json"])
	}

	baselineReport := filepath.Join(directory, "baseline-report.json")
	targetReport := filepath.Join(directory, "target-report.json")
	if err := os.WriteFile(baselineReport, []byte(before["json"]), 0o600); err != nil {
		t.Fatal(err)
	}
	targetJSON := runMetricsOutput(t, []string{"report", "--input", targetJournal, "--format", "json"})
	if err := os.WriteFile(targetReport, []byte(targetJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	compareArgs := []string{"compare", "--baseline", baselineReport, "--target", targetReport, "--format", "json"}
	firstComparison := runMetricsOutput(t, compareArgs)
	secondComparison := runMetricsOutput(t, compareArgs)
	if firstComparison != secondComparison {
		t.Fatalf("nondeterministic comparison:\n%s\n%s", firstComparison, secondComparison)
	}
	var comparison agentmetrics.ComparisonReport
	if err := json.Unmarshal([]byte(firstComparison), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.Baseline.Name != "pi" || comparison.Target.Name != "pi" || comparison.Delta.Metrics["elapsedMsMedian"].Absolute != -1000 {
		t.Fatalf("same-agent report comparison = %#v", comparison)
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
