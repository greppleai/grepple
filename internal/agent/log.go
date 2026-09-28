package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logDirectoryEnv = "GREPPLE_ASK_LOG_DIR"

// Log is a concurrency-safe JSONL event sink for one agent session.
type Log struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	path     string
	sequence uint64
	disabled bool
}

// LogOptions configure persistence and retention for agent logs.
type LogOptions struct {
	Enabled   bool
	Retention time.Duration
	now       func() time.Time
}

type logEvent struct {
	Schema   string    `json:"schema"`
	Sequence uint64    `json:"sequence"`
	Time     time.Time `json:"time"`
	Type     string    `json:"type"`
	Data     any       `json:"data,omitempty"`
}

// NewLogWithOptions creates a log using the supplied persistence policy.
func NewLogWithOptions(options LogOptions) (*Log, error) {
	if options.now == nil {
		options.now = time.Now
	}
	if options.Retention <= 0 {
		options.Retention = 7 * 24 * time.Hour
	}
	directory, err := logDirectory()
	if err != nil {
		return nil, err
	}
	if err := cleanExpiredLogs(directory, options.now().Add(-options.Retention)); err != nil {
		return nil, err
	}
	if !options.Enabled {
		return &Log{disabled: true}, nil
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create ask log directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("secure ask log directory: %w", err)
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return nil, fmt.Errorf("name ask log: %w", err)
	}
	name := options.now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix) + ".jsonl"
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create ask log: %w", err)
	}
	return &Log{file: file, encoder: json.NewEncoder(file), path: path}, nil
}

func logDirectory() (string, error) {
	if directory := os.Getenv(logDirectoryEnv); directory != "" {
		return directory, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "ask-logs"), nil
}

func cleanExpiredLogs(directory string, cutoff time.Time) error {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect ask log directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !isManagedLogName(entry.Name()) {
			continue
		}
		information, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect ask log %s: %w", entry.Name(), err)
		}
		if information.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				return fmt.Errorf("remove expired ask log %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func isManagedLogName(name string) bool {
	if !strings.HasSuffix(name, ".jsonl") {
		return false
	}
	base := strings.TrimSuffix(name, ".jsonl")
	timestamp, suffix, found := strings.Cut(base, "-")
	if !found || len(suffix) != 12 {
		return false
	}
	if _, err := time.Parse("20060102T150405.000000000Z", timestamp); err != nil {
		return false
	}
	_, err := hex.DecodeString(suffix)
	return err == nil
}

// Path returns the JSONL path, or an empty string when logging is disabled.
func (l *Log) Path() string { return l.path }

// Record appends one semantic event to the session log.
func (l *Log) Record(eventType string, data any) error {
	if l == nil || l.disabled {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sequence++
	event := logEvent{Schema: "grepple-ask-log-v1", Sequence: l.sequence, Time: time.Now().UTC(), Type: eventType, Data: data}
	if err := l.encoder.Encode(event); err != nil {
		return fmt.Errorf("write ask log %s: %w", l.path, err)
	}
	return nil
}

// Close durably flushes and closes the log.
func (l *Log) Close() error {
	if l == nil || l.disabled {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.file.Sync(); err != nil {
		_ = l.file.Close()
		return fmt.Errorf("sync ask log %s: %w", l.path, err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close ask log %s: %w", l.path, err)
	}
	return nil
}
