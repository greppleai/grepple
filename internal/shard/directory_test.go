package shard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryPersistAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "directory.json")
	d := loadDirectory(path)
	if _, ok := d.head("owner/repo"); ok {
		t.Fatal("empty directory should have no entries")
	}
	if err := d.set("owner/repo", "abc123"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := d.set("owner/other", "def456"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// set with the same head is a no-op but must not error.
	if err := d.set("owner/repo", "abc123"); err != nil {
		t.Fatalf("idempotent set: %v", err)
	}

	// Reload from disk: state must round-trip.
	reloaded := loadDirectory(path)
	if h, ok := reloaded.head("owner/repo"); !ok || h != "abc123" {
		t.Fatalf("reloaded owner/repo head=%q ok=%v", h, ok)
	}
	if h, ok := reloaded.head("owner/other"); !ok || h != "def456" {
		t.Fatalf("reloaded owner/other head=%q ok=%v", h, ok)
	}

	// Remove persists too.
	if err := reloaded.remove("owner/repo"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := loadDirectory(path).head("owner/repo"); ok {
		t.Fatal("removed repo should not reload")
	}
}

func TestLoadDirectoryMissingAndCorrupt(t *testing.T) {
	// Missing file -> empty directory, no error.
	d := loadDirectory(filepath.Join(t.TempDir(), "nope.json"))
	if d == nil || len(d.entries) != 0 {
		t.Fatal("missing directory should load empty")
	}
	// Corrupt file -> empty directory, no crash.
	path := filepath.Join(t.TempDir(), "directory.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := loadDirectory(path); d == nil || len(d.entries) != 0 {
		t.Fatal("corrupt directory should load empty")
	}
}

// nilDirectory methods must be safe (shard may have no directory in tests).
func TestNilDirectorySafe(t *testing.T) {
	var d *directory
	if _, ok := d.head("x"); ok {
		t.Fatal("nil head")
	}
	if err := d.set("x", "y"); err != nil {
		t.Fatalf("nil set: %v", err)
	}
	if err := d.remove("x"); err != nil {
		t.Fatalf("nil remove: %v", err)
	}
}
