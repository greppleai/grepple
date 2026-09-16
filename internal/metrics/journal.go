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

	EventRunStart   = "run_start"
	EventAssistant  = "assistant"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventCommand    = "command"
	EventCompaction = "compaction"
	EventRunEnd     = "run_end"

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

// MarshalJournalData converts typed event data into canonical JSON for a JournalEvent.
func MarshalJournalData(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode metrics event data: %w", err)
	}
	return data, nil
}

// AppendJournalEvent validates and appends one complete JSONL record under an OS file lock.
func AppendJournalEvent(path string, event JournalEvent) error {
	if err := validateJournalEvent(event); err != nil {
		return err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode metrics event: %w", err)
	}
	if len(line)+1 > maxJournalLineBytes {
		return fmt.Errorf("metrics event exceeds %d bytes", maxJournalLineBytes)
	}
	line = append(line, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create metrics journal directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure metrics journal directory: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open metrics journal lock: %w", err)
	}
	defer lock.Close()
	if err := lockJournalFile(lock, true); err != nil {
		return fmt.Errorf("lock metrics journal: %w", err)
	}
	defer unlockJournalFile(lock) //nolint:errcheck // best-effort release during return
	if err := validateJournalAppend(path, event); err != nil {
		return err
	}
	if information, statErr := os.Stat(path); statErr == nil && information.Size()+int64(len(line)) > maxJournalBytes {
		return fmt.Errorf("metrics journal exceeds %d bytes", maxJournalBytes)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat metrics journal: %w", statErr)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open metrics journal: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		file.Close()
		return fmt.Errorf("secure metrics journal: %w", err)
	}
	if _, err := file.Write(line); err != nil {
		file.Close()
		return fmt.Errorf("append metrics journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync metrics journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close metrics journal: %w", err)
	}
	return nil
}

// ReadJournal reads and validates a bounded Grepple metrics JSONL journal.
func ReadJournal(path string) ([]JournalEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	lock, err := os.Open(path + ".lock")
	if errors.Is(err, os.ErrNotExist) {
		return decodeJournal(file)
	}
	if err != nil {
		return nil, fmt.Errorf("open metrics journal lock: %w", err)
	}
	defer lock.Close()
	if err := lockJournalFile(lock, false); err != nil {
		return nil, fmt.Errorf("lock metrics journal for reading: %w", err)
	}
	defer unlockJournalFile(lock) //nolint:errcheck // best-effort release during return
	return decodeJournal(file)
}

func validateJournalAppend(path string, candidate JournalEvent) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		if candidate.Event != EventRunStart {
			return fmt.Errorf("%s appears before run_start", candidate.Event)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("open metrics journal for validation: %w", err)
	}
	events, decodeErr := decodeJournal(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return decodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if len(events) >= maxJournalEvents {
		return fmt.Errorf("metrics journal exceeds %d events", maxJournalEvents)
	}
	started := false
	ended := false
	for _, event := range events {
		if event.EventID == candidate.EventID {
			return fmt.Errorf("duplicate eventId %q", candidate.EventID)
		}
		if event.RunID != candidate.RunID {
			continue
		}
		if event.Event == EventRunStart {
			started = true
		}
		if event.Event == EventRunEnd {
			ended = true
		}
	}
	if candidate.Event == EventRunStart {
		if started {
			return fmt.Errorf("duplicate run_start for run %q", candidate.RunID)
		}
		return nil
	}
	if !started {
		return fmt.Errorf("%s appears before run_start", candidate.Event)
	}
	if ended {
		return fmt.Errorf("%s appears after run_end", candidate.Event)
	}
	return nil
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

// DiscoverJournals returns sorted, deduplicated journal paths. The default is ~/.grepple/metrics.
func DiscoverJournals(inputs []string, home string) ([]string, error) {
	explicit := len(inputs) > 0
	if !explicit {
		inputs = []string{filepath.Join(home, ".grepple", "metrics")}
	}
	seen := make(map[string]bool)
	for _, input := range inputs {
		if err := discoverJournalInput(filepath.Clean(input), seen); err != nil {
			if !explicit && errors.Is(err, os.ErrNotExist) {
				continue
			}
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
