package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

func TestWriteMetricsReportIsBounded(t *testing.T) {
	groups := make([]agentmetrics.Group, 2_000)
	for index := range groups {
		groups[index] = agentmetrics.Group{
			Name:            strings.Repeat("group", 8),
			SampleSize:      1,
			UsageSampleSize: 1,
			Tokens:          &agentmetrics.Distribution{P50: 1, P90: 2},
			Cost:            &agentmetrics.Distribution{P50: 0.1, P90: 0.2},
			ElapsedMS:       agentmetrics.Distribution{P50: 10, P90: 20},
		}
	}
	report := agentmetrics.Report{Generated: time.Now(), Groups: groups}
	var renderErr error
	output := captureStdout(t, func() {
		renderErr = writeMetricsReport(report, "task")
	})
	if !errors.Is(renderErr, errOutputTruncated) {
		t.Fatalf("render error = %v", renderErr)
	}
	if len(output) > DefaultTextOutputBytes || !strings.Contains(output, "truncated") {
		t.Fatalf("bounded output length = %d", len(output))
	}
}

func TestIncludeMetricRunFiltersControlledDimensions(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	run := agentmetrics.Run{
		Repository:     "repo",
		Provider:       "provider",
		Model:          "model",
		TaskID:         "task",
		AssignedCohort: "control",
		Complete:       true,
		StartedAt:      start,
		EndedAt:        start.Add(time.Minute),
	}
	options := metricsOptions{
		repository:   "repo",
		model:        "provider/model",
		task:         "task",
		cohort:       "control",
		onlyComplete: true,
		since:        start.Add(30 * time.Second),
		until:        start.Add(30 * time.Second),
	}
	if !includeMetricRun(run, options) {
		t.Fatal("matching controlled run was filtered out")
	}
	options.cohort = "grepple"
	if includeMetricRun(run, options) {
		t.Fatal("mismatched cohort was included")
	}
}

func TestMetricsHelpDescribesJSONLAnalysisWorkflow(t *testing.T) {
	for _, want := range []string{"metrics report", "metrics compare", "--input", "--baseline", "--target"} {
		if !strings.Contains(metricsHelp, want) {
			t.Errorf("metrics help lacks %q", want)
		}
	}
	for _, forbidden := range []string{"metrics start", "metrics record", "metrics status", "metrics end", "metrics export", "~/.grepple/metrics", "GREPPLE_METRICS_DIR", "active run", "Pi", "session", "--leaf", "~/.pi"} {
		if strings.Contains(metricsHelp, forbidden) {
			t.Errorf("metrics help contains obsolete term %q", forbidden)
		}
	}
}

func TestMetricsCommandsRequireExplicitJSONLInputs(t *testing.T) {
	missingInputCommands := [][]string{{"report"}, {"compare", "--baseline", "control", "--target", "grepple"}}
	for _, args := range missingInputCommands {
		if err := runMetrics(args); err == nil || !strings.Contains(err.Error(), "requires --input") {
			t.Fatalf("metrics %s error = %v", args[0], err)
		}
	}
	for _, command := range []string{"start", "record", "status", "end", "export"} {
		err := runMetrics([]string{command})
		if err == nil || !strings.Contains(err.Error(), "unknown metrics command") {
			t.Errorf("metrics %s error = %v", command, err)
		}
	}
}

func TestMetricsOutputsLeaveUnavailableUsageBlankOrUnknown(t *testing.T) {
	report := agentmetrics.Report{
		Runs:   []agentmetrics.Run{{RunID: "run-1", Missing: []string{"agent_turns", "model_usage"}}},
		Groups: []agentmetrics.Group{{Name: "control", SampleSize: 1, ElapsedMS: agentmetrics.Distribution{}}},
	}
	human := captureStdout(t, func() {
		if err := writeMetricsReport(report, "cohort"); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(human, "\tunknown\tunknown\tunknown\t") {
		t.Fatalf("human report = %q", human)
	}
	csvOutput := captureStdout(t, func() {
		if err := exportMetrics(report, "csv"); err != nil {
			t.Fatal(err)
		}
	})
	rows := strings.Split(strings.TrimSpace(csvOutput), "\n")
	fields := strings.Split(rows[1], ",")
	if fields[10] != "" || fields[13] != "" || fields[14] != "" {
		t.Fatalf("unavailable CSV fields = turns %q, tokens %q, cost %q", fields[10], fields[13], fields[14])
	}
}
