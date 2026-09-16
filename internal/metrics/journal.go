package metrics

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// JournalSchema is the version-one Grepple-owned metrics event schema.
	JournalSchema = "grepple-metrics-event-v1"

	// EventRunStart identifies the event that establishes a run and its correlation metadata.
	EventRunStart = "run_start"
	// EventAssistant identifies normalized assistant-turn and model-usage evidence.
	EventAssistant = "assistant"
	// EventToolCall identifies a normalized tool invocation without content-bearing arguments.
	EventToolCall = "tool_call"
	// EventToolResult identifies the normalized outcome and aggregate volume of a tool invocation.
	EventToolResult = "tool_result"
	// EventCommand identifies privacy-safe Grepple activity supplied by a journal producer.
	EventCommand = "command"
	// EventCompaction identifies an observed context-compaction boundary.
	EventCompaction = "compaction"
	// EventRunEnd identifies explicit run completion and evaluation evidence.
	EventRunEnd = "run_end"

	maxJournalLineBytes = 1 << 20
	maxJournalBytes     = 64 << 20
	maxJournalEvents    = 100_000
)

var journalEventTypes = map[string]bool{
	EventRunStart: true, EventAssistant: true, EventToolCall: true,
	EventToolResult: true, EventCommand: true, EventCompaction: true, EventRunEnd: true,
}

// JournalEvent is one privacy-safe event in a Grepple metrics JSONL journal.
type JournalEvent struct {
	Schema  string          `json:"schema"`
	EventID string          `json:"eventId"`
	Time    time.Time       `json:"time"`
	RunID   string          `json:"runId"`
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
}

// ReadJournal reads and validates a bounded Grepple metrics JSONL journal.
func ReadJournal(path string) ([]JournalEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return decodeJournal(file)
}

func decodeJournal(reader io.Reader) ([]JournalEvent, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxJournalLineBytes)
	events := make([]JournalEvent, 0, 64)
	seen := make(map[string]bool)
	line := 0
	totalBytes := 0
	for scanner.Scan() {
		line++
		totalBytes += len(scanner.Bytes()) + 1
		if totalBytes > maxJournalBytes {
			return nil, fmt.Errorf("metrics journal exceeds %d bytes", maxJournalBytes)
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event JournalEvent
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			return nil, fmt.Errorf("decode metrics journal line %d: %w", line, err)
		}
		if err := validateJournalEvent(event); err != nil {
			return nil, fmt.Errorf("metrics journal line %d: %w", line, err)
		}
		if seen[event.EventID] {
			return nil, fmt.Errorf("metrics journal line %d: duplicate eventId %q", line, event.EventID)
		}
		seen[event.EventID] = true
		events = append(events, event)
		if len(events) > maxJournalEvents {
			return nil, fmt.Errorf("metrics journal exceeds %d events", maxJournalEvents)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan metrics journal: %w", err)
	}
	return events, nil
}

func validateJournalEvent(event JournalEvent) error {
	if event.Schema != JournalSchema {
		return fmt.Errorf("unsupported metrics journal schema %q", event.Schema)
	}
	if strings.TrimSpace(event.EventID) == "" {
		return errors.New("metrics eventId is required")
	}
	if event.Time.IsZero() {
		return errors.New("metrics event time is required")
	}
	if strings.TrimSpace(event.RunID) == "" {
		return errors.New("metrics runId is required")
	}
	if !journalEventTypes[event.Event] {
		return fmt.Errorf("unsupported metrics event %q", event.Event)
	}
	if len(event.Data) == 0 || !json.Valid(event.Data) {
		return errors.New("metrics event data must be valid JSON")
	}
	if _, err := DecodeEventData(event.Event, event.Data); err != nil {
		return err
	}
	return nil
}

// DiscoverJournals returns sorted, deduplicated journal paths from explicit inputs.
func DiscoverJournals(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, errors.New("metrics input is required")
	}
	seen := make(map[string]bool)
	for _, input := range inputs {
		if err := discoverJournalInput(filepath.Clean(input), seen); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func discoverJournalInput(input string, seen map[string]bool) error {
	information, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !information.IsDir() {
		if filepath.Ext(input) != ".jsonl" {
			return fmt.Errorf("metrics input %q is not a .jsonl file", input)
		}
		seen[input] = true
		return nil
	}
	return filepath.WalkDir(input, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		seen[path] = true
		return nil
	})
}
