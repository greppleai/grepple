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
		renderErr = writeMetricsReport(report)
	})
	if !errors.Is(renderErr, errOutputTruncated) {
		t.Fatalf("render error = %v", renderErr)
	}
	if len(output) > DefaultTextOutputBytes || !strings.Contains(output, "truncated") {
		t.Fatalf("bounded output length = %d", len(output))
	}
}

func TestMetricsHelpDescribesReportFileComparisonWorkflow(t *testing.T) {
	for _, want := range []string{"metrics report", "metrics compare", "--input", "--baseline", "--target", "JSON report"} {
		if !strings.Contains(metricsHelp, want) {
			t.Errorf("metrics help lacks %q", want)
		}
	}
	for _, forbidden := range []string{"metrics start", "metrics record", "metrics status", "metrics end", "metrics export", "~/.grepple/metrics", "GREPPLE_METRICS_DIR", "active run", "session", "--leaf", "~/.pi", "--group-by", "--repository", "--model", "--task", "--cohort", "--since", "--until", "--complete"} {
		if strings.Contains(metricsHelp, forbidden) {
			t.Errorf("metrics help contains obsolete term %q", forbidden)
		}
	}
}

func TestMetricsOptionsRejectRemovedDimensions(t *testing.T) {
	for _, option := range []string{"--group-by", "--repository", "--model", "--task", "--cohort", "--since", "--until", "--complete"} {
		t.Run(option, func(t *testing.T) {
			args := []string{"--input", "runs.jsonl", option}
			if option != "--complete" {
				args = append(args, "value")
			}
			if _, _, err := parseMetricsOptions("report", args); err == nil {
				t.Fatalf("removed option %s was accepted", option)
			}
		})
	}
	for _, option := range []string{"--baseline", "--target"} {
		if _, _, err := parseMetricsOptions("report", []string{"--input", "runs.jsonl", option, "pi"}); err == nil {
			t.Fatalf("report accepted compare-only option %s", option)
		}
	}
}

func TestMetricsCommandsRequireTheirExplicitEvidenceInputs(t *testing.T) {
	if err := runMetrics([]string{"report"}); err == nil || !strings.Contains(err.Error(), "requires --input") {
		t.Fatalf("metrics report error = %v", err)
	}
	if err := runMetrics([]string{"compare", "--baseline", "baseline.json"}); err == nil || !strings.Contains(err.Error(), "requires --baseline and --target") {
		t.Fatalf("metrics compare error = %v", err)
	}
	if _, _, err := parseMetricsOptions("compare", []string{"--input", "runs.jsonl", "--baseline", "baseline.json", "--target", "target.json"}); err == nil {
		t.Fatal("metrics compare accepted JSONL --input")
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
		Runs:   []agentmetrics.Run{{RunID: "run-1", Agent: "pi", Missing: []string{"agent_turns", "assistant_usage"}}},
		Groups: []agentmetrics.Group{{Name: "pi", SampleSize: 1, ElapsedMS: agentmetrics.Distribution{}}},
	}
	human := captureStdout(t, func() {
		if err := writeMetricsReport(report); err != nil {
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
	if fields[5] != "" || fields[8] != "" || fields[9] != "" {
		t.Fatalf("unavailable CSV fields = turns %q, tokens %q, cost %q", fields[5], fields[8], fields[9])
	}
}
