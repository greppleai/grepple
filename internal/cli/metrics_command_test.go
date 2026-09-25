package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetricsCommandIsRegistered(t *testing.T) {
	fixture := filepath.Join("..", "metrics", "testdata", "comprehensive-v1.jsonl")
	output := captureStdout(t, func() {
		if err := Run([]string{"--no-spill", "metrics", "report", "--input", fixture, "--format", "json"}); err != nil {
			t.Fatal(err)
		}
	})
	var report struct {
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal([]byte(output), &report); err != nil || len(report.Runs) == 0 {
		t.Fatalf("metrics report output = %q, err = %v", output, err)
	}
	help := captureStdout(t, func() {
		if err := Run([]string{"help", "metrics"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"report", "compare", "Grepple does not collect"} {
		if !strings.Contains(help, want) {
			t.Fatalf("metrics help lacks %q:\n%s", want, help)
		}
	}
	if err := Run([]string{"metrics", "report", "--remote", "--input", fixture}); err == nil || !strings.Contains(err.Error(), "metrics is local-only") {
		t.Fatalf("remote metrics error = %v", err)
	}
}
