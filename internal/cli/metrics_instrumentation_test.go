package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

func TestActiveMetricsRunRecordsGreppleCommandWithoutArguments(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(metricsDirectoryEnv, directory)
	if err := runMetrics([]string{"start", "--run", "run-observed", "--event-id", "start-1", "--at", "2026-09-16T08:00:00Z", "--task", "task-1", "--cohort", "grepple"}); err != nil {
		t.Fatal(err)
	}
	searchRoot := t.TempDir()
	secret := "content-bearing-secret-query"
	if err := os.WriteFile(filepath.Join(searchRoot, "sample.txt"), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var commandErr error
	output := captureStdout(t, func() {
		commandErr = Run([]string{"-F", secret, searchRoot, "--no-anchors"})
	})
	if commandErr != nil {
		t.Fatal(commandErr)
	}
	if !strings.Contains(output, secret) {
		t.Fatalf("search output changed: %q", output)
	}
	path := filepath.Join(directory, "runs", "run-observed.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), searchRoot) {
		t.Fatalf("journal leaked command content: %s", raw)
	}
	events, err := agentmetrics.ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Event != agentmetrics.EventCommand {
		t.Fatalf("events = %#v", events)
	}
	decoded, err := agentmetrics.DecodeEventData(events[1].Event, events[1].Data)
	if err != nil {
		t.Fatal(err)
	}
	command := decoded.(*agentmetrics.CommandData)
	if command.Name != "search" || !command.Success || command.GreppleMode != "search" {
		t.Fatalf("command data = %#v", command)
	}
}

func TestMetricsCommandsDoNotInstrumentThemselves(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(metricsDirectoryEnv, directory)
	if err := runMetrics([]string{"start", "--run", "run-1", "--event-id", "start-1", "--at", "2026-09-16T08:00:00Z", "--task", "task-1", "--cohort", "control"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"metrics", "status"}); err != nil {
		t.Fatal(err)
	}
	events, err := agentmetrics.ReadJournal(filepath.Join(directory, "runs", "run-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("metrics status self-recorded: %#v", events)
	}
}
