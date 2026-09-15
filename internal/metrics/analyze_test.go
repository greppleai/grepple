package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeFileAggregatesUsageToolsAndMilestones(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	lines := []string{
		`{"type":"session","version":3,"id":"session-1","timestamp":"2026-01-01T00:00:00Z","cwd":"/work/repo"}`,
		`{"type":"custom","id":"start","parentId":null,"timestamp":"2026-01-01T00:00:00Z","customType":"grepple-metrics-v1","data":{"schema":1,"event":"run_start","runId":"run-1","taskId":"task-1","assignedCohort":"grepple"}}`,
		`{"type":"message","id":"assistant-1","parentId":"start","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","provider":"test","model":"model","timestamp":1767225601000,"usage":{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"totalTokens":18,"cost":{"total":0.02}},"content":[{"type":"toolCall","id":"call-search","name":"bash","arguments":{"command":"grepple -n ParseDocument parser"}},{"type":"toolCall","id":"call-read","name":"functions.read","arguments":{"path":"parser/parser.go"}}]}}`,
		`{"type":"message","id":"result-search","parentId":"assistant-1","timestamp":"2026-01-01T00:00:02Z","message":{"role":"toolResult","toolCallId":"call-search","toolName":"bash","content":[{"type":"text","text":"parser/parser.go:10"}],"details":{"exitCode":0},"isError":false}}`,
		`{"type":"message","id":"result-read","parentId":"result-search","timestamp":"2026-01-01T00:00:03Z","message":{"role":"toolResult","toolCallId":"call-read","toolName":"functions.read","content":[{"type":"text","text":"line one\nline two"}],"isError":false}}`,
		`{"type":"message","id":"assistant-2","parentId":"result-read","timestamp":"2026-01-01T00:00:04Z","message":{"role":"assistant","provider":"test","model":"model","timestamp":1767225604000,"usage":{"input":20,"output":10,"cacheRead":0,"cacheWrite":0,"totalTokens":30,"cost":{"total":0.03}},"content":[{"type":"toolCall","id":"call-edit","name":"functions.edit","arguments":{"path":"parser/parser.go","changes":[{"content_lines":["replacement"]}]}},{"type":"toolCall","id":"call-test","name":"functions.bash","arguments":{"command":"go test ./parser"}}]}}`,
		`{"type":"message","id":"result-edit","parentId":"assistant-2","timestamp":"2026-01-01T00:00:05Z","message":{"role":"toolResult","toolCallId":"call-edit","toolName":"functions.edit","content":[{"type":"text","text":"ok"}],"isError":false}}`,
		`{"type":"message","id":"result-test","parentId":"result-edit","timestamp":"2026-01-01T00:00:06Z","message":{"role":"toolResult","toolCallId":"call-test","toolName":"functions.bash","content":[{"type":"text","text":"PASS"}],"details":{"exitCode":0},"isError":false}}`,
		`{"type":"custom","id":"end","parentId":"result-test","timestamp":"2026-01-01T00:00:07Z","customType":"grepple-metrics-v1","data":{"schema":1,"event":"run_end","runId":"run-1","outcome":"success","firstEditSurvived":true}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	runs, err := AnalyzeFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	run := runs[0]
	if run.Usage.TotalTokens != 48 || run.Tools.Total != 4 || run.Tools.Grepple != 1 {
		t.Fatalf("unexpected totals: %#v", run)
	}
	if run.FirstAttemptedMutation == nil || run.FirstAttemptedMutation.CallsBefore != 2 || run.FirstAttemptedMutation.CumulativeUsage.TotalTokens != 48 {
		t.Fatalf("first mutation = %#v", run.FirstAttemptedMutation)
	}
	if run.FirstPassingTest == nil || run.Outcome.Status != "success" || !run.Complete {
		t.Fatalf("completion metrics missing: %#v", run)
	}
	if run.DistinctInspectedFiles != 1 || run.DistinctEditedFiles != 1 || run.AddedLines != 1 {
		t.Fatalf("file/churn metrics unexpected: %#v", run)
	}
}

func TestAnalyzeFileRejectsFutureAnnotationSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	input := `{"type":"session","version":3,"id":"s","cwd":"/repo"}
{"type":"custom","id":"x","parentId":null,"timestamp":"2026-01-01T00:00:00Z","customType":"grepple-metrics-v1","data":{"schema":2,"event":"run_start"}}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeFile(path, "")
	if err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("error = %v", err)
	}
}
