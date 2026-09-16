package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestJournalRoundTripUsesPrivateAppendOnlyJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "run.jsonl")
	event := JournalEvent{
		Schema:  JournalSchema,
		EventID: "event-1",
		Time:    time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC),
		RunID:   "run-1",
		Event:   EventRunStart,
		Data:    json.RawMessage(`{"taskId":"task-1","repository":"repo","revision":"abc","assignedCohort":"control"}`),
	}
	if err := AppendJournalEvent(path, event); err != nil {
		t.Fatal(err)
	}
	events, err := ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "event-1" || string(events[0].Data) != `{"taskId":"task-1","repository":"repo","revision":"abc","assignedCohort":"control"}` {
		t.Fatalf("unexpected events: %#v", events)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("journal mode = %o, want 600", got)
	}
	info, err = os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("journal directory mode = %o, want 700", got)
	}
}

func TestJournalRejectsInvalidAndOversizedRecords(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "future schema", content: `{"schema":"grepple-metrics-event-v2","eventId":"e","time":"2026-09-16T08:00:00Z","runId":"r","event":"run_start","data":{}}` + "\n"},
		{name: "unknown event", content: `{"schema":"grepple-metrics-event-v1","eventId":"e","time":"2026-09-16T08:00:00Z","runId":"r","event":"mystery","data":{}}` + "\n"},
		{name: "truncated", content: `{"schema":"grepple-metrics-event-v1"`},
		{name: "oversized", content: string(make([]byte, maxJournalLineBytes+1))},
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

func TestAppendJournalEventRejectsDuplicateAndInvalidLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	start := JournalEvent{
		Schema: JournalSchema, EventID: "start", Time: time.Unix(1, 0).UTC(), RunID: "run-1", Event: EventRunStart,
		Data: json.RawMessage(`{"taskId":"task-1","assignedCohort":"control"}`),
	}
	if err := AppendJournalEvent(path, JournalEvent{Schema: JournalSchema, EventID: "early", Time: time.Unix(0, 0).UTC(), RunID: "run-1", Event: EventCommand, Data: json.RawMessage(`{"name":"search"}`)}); err == nil {
		t.Fatal("accepted command before run_start")
	}
	if err := AppendJournalEvent(path, start); err != nil {
		t.Fatal(err)
	}
	if err := AppendJournalEvent(path, start); err == nil {
		t.Fatal("accepted duplicate event ID")
	}
	end := JournalEvent{Schema: JournalSchema, EventID: "end", Time: time.Unix(2, 0).UTC(), RunID: "run-1", Event: EventRunEnd, Data: json.RawMessage(`{"outcome":"success"}`)}
	if err := AppendJournalEvent(path, end); err != nil {
		t.Fatal(err)
	}
	if err := AppendJournalEvent(path, JournalEvent{Schema: JournalSchema, EventID: "late", Time: time.Unix(3, 0).UTC(), RunID: "run-1", Event: EventCommand, Data: json.RawMessage(`{"name":"search"}`)}); err == nil {
		t.Fatal("accepted event after run_end")
	}
}

func TestConcurrentJournalAppendsProduceWholeRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.jsonl")
	start := JournalEvent{Schema: JournalSchema, EventID: "start", Time: time.Unix(0, 0).UTC(), RunID: "run-1", Event: EventRunStart, Data: json.RawMessage(`{"taskId":"task-1","assignedCohort":"control"}`)}
	if err := AppendJournalEvent(path, start); err != nil {
		t.Fatal(err)
	}
	const count = 24
	var group sync.WaitGroup
	errors := make(chan error, count)
	for index := 0; index < count; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			event := JournalEvent{
				Schema: JournalSchema, EventID: fmt.Sprintf("event-%02d", index),
				Time: time.Unix(int64(index), 0).UTC(), RunID: "run-1", Event: EventCommand,
				Data: json.RawMessage(`{"name":"search"}`),
			}
			errors <- AppendJournalEvent(path, event)
		}(index)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	events, err := ReadJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != count+1 {
		t.Fatalf("event count = %d, want %d", len(events), count+1)
	}
	seen := make(map[string]bool, count+1)
	for _, event := range events {
		seen[event.EventID] = true
	}
	if len(seen) != count+1 {
		t.Fatalf("unique event IDs = %d, want %d", len(seen), count+1)
	}
}

func TestDiscoverJournalsIsSortedAndDeduplicated(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".grepple", "metrics")
	for _, name := range []string{"z.jsonl", "nested/a.jsonl", "ignore.txt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := DiscoverJournals([]string{root, filepath.Join(root, "z.jsonl")}, home)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "nested/a.jsonl"), filepath.Join(root, "z.jsonl")}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	defaults, err := DiscoverJournals(nil, home)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(defaults) != fmt.Sprint(want) {
		t.Fatalf("default paths = %v, want %v", defaults, want)
	}
}
