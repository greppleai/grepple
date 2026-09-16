package metrics

import (
	"strings"
	"testing"
)

func TestDecodeSessionSelectsLatestBranch(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","version":3,"id":"session-1","timestamp":"2026-01-01T00:00:00Z","cwd":"/repo"}`,
		`{"type":"message","id":"root","parentId":null,"timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"task"}}`,
		`{"type":"message","id":"abandoned","parentId":"root","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[],"usage":{"totalTokens":999}}}`,
		`{"type":"branch_summary","id":"summary","parentId":"root","timestamp":"2026-01-01T00:00:03Z","fromId":"abandoned","summary":"redacted"}`,
		`{"type":"message","id":"leaf","parentId":"summary","timestamp":"2026-01-01T00:00:04Z","message":{"role":"assistant","content":[],"usage":{"totalTokens":10}}}`,
	}, "\n")
	branch, err := decodeSession(strings.NewReader(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(branch.Entries), 3; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	if branch.Entries[1].ID != "summary" {
		t.Fatalf("selected branch contains %q", branch.Entries[1].ID)
	}
}

func TestDecodeSessionSupportsLegacyLinearVersion(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session","id":"legacy","timestamp":"2026-01-01T00:00:00Z","cwd":"/repo"}`,
		`{"type":"message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"task"}}`,
		`{"type":"message","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","content":[],"usage":{"totalTokens":10}}}`,
	}, "\n")
	branch, err := decodeSession(strings.NewReader(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(branch.Entries), 2; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	if branch.Entries[0].ID == "" || branch.Entries[1].ID == "" {
		t.Fatal("legacy entries did not receive stable IDs")
	}
	if branch.Entries[0].ParentID != nil {
		t.Fatalf("first parent = %v, want nil", branch.Entries[0].ParentID)
	}
	if parent := branch.Entries[1].ParentID; parent == nil || *parent != branch.Entries[0].ID {
		t.Fatalf("second parent = %v, want %q", parent, branch.Entries[0].ID)
	}
}

func TestDecodeSessionRejectsFutureVersion(t *testing.T) {
	_, err := decodeSession(strings.NewReader(`{"type":"session","version":4,"id":"s"}`), "")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v", err)
	}
}

func TestDecodeSessionRejectsMissingParent(t *testing.T) {
	input := "{\"type\":\"session\",\"version\":3,\"id\":\"s\"}\n" +
		"{\"type\":\"message\",\"id\":\"leaf\",\"parentId\":\"missing\",\"message\":{}}"
	_, err := decodeSession(strings.NewReader(input), "")
	if err == nil || !strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("error = %v", err)
	}
}
