package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const askLogDirectoryEnv = "GREPPLE_ASK_LOG_DIR"

type askLog struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	path     string
	sequence uint64
}

type askLogEvent struct {
	Schema   string    `json:"schema"`
	Sequence uint64    `json:"sequence"`
	Time     time.Time `json:"time"`
	Type     string    `json:"type"`
	Data     any       `json:"data,omitempty"`
}

func newAskLog() (*askLog, error) {
	directory := os.Getenv(askLogDirectoryEnv)
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		directory = filepath.Join(home, ".grepple", "ask-logs")
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
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix) + ".jsonl"
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create ask log: %w", err)
	}
	return &askLog{file: file, encoder: json.NewEncoder(file), path: path}, nil
}

func (l *askLog) Path() string { return l.path }

func (l *askLog) Record(eventType string, data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sequence++
	event := askLogEvent{Schema: "grepple-ask-log-v1", Sequence: l.sequence, Time: time.Now().UTC(), Type: eventType, Data: data}
	if err := l.encoder.Encode(event); err != nil {
		return fmt.Errorf("write ask log %s: %w", l.path, err)
	}
	return nil
}

func (l *askLog) Close() error {
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
