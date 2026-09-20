package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogRetentionRemovesOnlyExpiredManagedLogs(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(logDirectoryEnv, directory)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	oldLog := filepath.Join(directory, "20260901T120000.000000000Z-001122334455.jsonl")
	recentLog := filepath.Join(directory, "20260915T120000.000000000Z-66778899aabb.jsonl")
	unrelated := filepath.Join(directory, "notes.jsonl")
	for _, path := range []string{oldLog, recentLog, unrelated} {
		if err := os.WriteFile(path, []byte("evidence\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldLog, now.Add(-8*24*time.Hour), now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentLog, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	log, err := NewLogWithOptions(LogOptions{Enabled: false, Retention: 7 * 24 * time.Hour, now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if log.Path() != "" {
		t.Fatalf("disabled log path=%q", log.Path())
	}
	if err := log.Record("ignored", nil); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("expired log still exists: %v", err)
	}
	for _, path := range []string{recentLog, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained file %s: %v", path, err)
		}
	}
}

func TestDisabledAskLogsDoNotCreateDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing")
	t.Setenv(logDirectoryEnv, directory)
	log, err := NewLogWithOptions(LogOptions{Enabled: false, Retention: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if log.Path() != "" {
		t.Fatalf("disabled log path=%q", log.Path())
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("disabled logging created directory: %v", err)
	}
}

func TestEnabledAskLogUsesPrivatePermissions(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(logDirectoryEnv, directory)
	log, err := NewLogWithOptions(LogOptions{Enabled: true, Retention: 7 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Record("test", map[string]string{"result": "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	information, err := os.Stat(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	if information.Mode().Perm() != 0o600 {
		t.Fatalf("log mode=%o", information.Mode().Perm())
	}
}
