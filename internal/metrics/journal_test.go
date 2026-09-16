package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validStartEvent = `{"schema":"grepple-metrics-event-v1","eventId":"start","time":"2026-09-16T08:00:00Z","runId":"run-1","event":"run_start","data":{"taskId":"task-1","assignedCohort":"control"}}`

func TestReadJournalValidatesDirectJSONLInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	content := validStartEvent + "\n" +
		`{"schema":"grepple-metrics-event-v1","eventId":"end","time":"2026-09-16T08:01:00Z","runId":"run-1","event":"run_end","data":{"outcome":"success"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	events, err := ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventID != "start" || events[1].Event != EventRunEnd {
		t.Fatalf("events = %#v", events)
	}
}

func TestReadJournalRejectsInvalidBoundedRecords(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "future schema", content: `{"schema":"grepple-metrics-event-v2","eventId":"e","time":"2026-09-16T08:00:00Z","runId":"r","event":"run_start","data":{}}` + "\n"},
		{name: "unknown event", content: `{"schema":"grepple-metrics-event-v1","eventId":"e","time":"2026-09-16T08:00:00Z","runId":"r","event":"mystery","data":{}}` + "\n"},
		{name: "unknown field", content: strings.TrimSuffix(validStartEvent, "}") + `,"secret":"value"}` + "\n"},
		{name: "truncated", content: `{"schema":"grepple-metrics-event-v1"`},
		{name: "oversized", content: string(make([]byte, maxJournalLineBytes+1))},
		{name: "duplicate", content: validStartEvent + "\n" + validStartEvent + "\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad.jsonl")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadJournal(path); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDiscoverJournalsRequiresExplicitSortedInputs(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z.jsonl", "nested/a.jsonl", "ignore.txt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := DiscoverJournals([]string{root, filepath.Join(root, "z.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "nested/a.jsonl"), filepath.Join(root, "z.jsonl")}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if _, err := DiscoverJournals(nil); err == nil {
		t.Fatal("DiscoverJournals accepted no inputs")
	}
	if _, err := DiscoverJournals([]string{filepath.Join(root, "ignore.txt")}); err == nil {
		t.Fatal("DiscoverJournals accepted non-JSONL input")
	}
}
