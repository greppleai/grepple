package pihooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCommandInvokesGrepple(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"grepple -F Widget .", true},
		{"cd /workspace && ./bin/grepple --at main.go:10", true},
		{"HOME=/tmp ../grepple/bin/grepple --outline main.go", true},
		{`sh -c 'grepple -F Widget .'`, true},
		{"printf grepple", false},
		{"echo ./bin/grepple", false},
		{"go test ./...", false},
	}
	for _, test := range tests {
		if got := CommandInvokesGrepple(test.command); got != test.want {
			t.Errorf("CommandInvokesGrepple(%q)=%v want %v", test.command, got, test.want)
		}
	}
}

func TestHandleContextGuardOmitsOnlyPreviouslyEmittedBlocks(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	first := strings.Repeat("first source line\n", 12) + "\n"
	second := strings.Repeat("second source line\n", 12) + "\n"
	payload := contextGuardPayload(t, "session-a", "grepple --at source.go:1", first+second)
	if output := HandleContextGuard(payload); len(output) != 0 {
		t.Fatalf("first emission was changed: %s", output)
	}
	cachePath := filepath.Join(directory, "session-a.json")
	cacheContent, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache was not stored: %v", err)
	}
	if strings.Contains(string(cacheContent), "source line") {
		t.Fatalf("cache persisted source content: %s", cacheContent)
	}

	third := strings.Repeat("third source line\n", 12) + "\n"
	guarded := decodeUpdatedToolOutput(t, HandleContextGuard(contextGuardPayload(t, "session-a", "grepple --at source.go:1", first+third)))
	if !strings.Contains(guarded, "omitted 1 unchanged output block") || strings.Contains(guarded, "first source line") {
		t.Fatalf("unchanged block was not omitted: %q", guarded)
	}
	if !strings.Contains(guarded, "third source line") {
		t.Fatalf("new block was omitted: %q", guarded)
	}
}

func TestHandleContextGuardSeparatesSessionsAndInvalidatesAfterCompaction(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	output := strings.Repeat("stable external dependency source\n", 8)
	first := contextGuardPayload(t, "session-a", "grepple --at dependency.ts:1", output)
	if got := HandleContextGuard(first); len(got) != 0 {
		t.Fatalf("first output changed: %s", got)
	}
	if got := HandleContextGuard(contextGuardPayload(t, "session-b", "grepple --at dependency.ts:1", output)); len(got) != 0 {
		t.Fatalf("different session reused cache: %s", got)
	}
	if got := HandleContextGuard(first); len(got) == 0 {
		t.Fatal("same session did not suppress repeated output")
	}
	HandleContextInvalidation([]byte(`{"session_id":"session-a","hook_event_name":"PostCompact"}`))
	if got := HandleContextGuard(first); len(got) != 0 {
		t.Fatalf("compaction did not reset cache: %s", got)
	}
}

func TestHandleContextGuardDeduplicatesNativeReadOutput(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	output := strings.Repeat("abc│42│unchanged local source line\n", 8)
	payload := contextGuardPayloadForTool(t, "read-session", "Read", map[string]any{"path": "source.go"}, output)
	if got := HandleContextGuard(payload); len(got) != 0 {
		t.Fatalf("first Read output changed: %s", got)
	}
	guarded := decodeUpdatedToolOutput(t, HandleContextGuard(payload))
	if !strings.Contains(guarded, "already emitted in this session") || strings.Contains(guarded, "local source line") {
		t.Fatalf("repeated Read output was not omitted: %q", guarded)
	}
}

func TestHandleContextGuardFailsOpenForUnsupportedOutput(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	text := strings.Repeat("source\n", 30)
	cases := [][]byte{
		[]byte("not-json"),
		contextGuardPayload(t, "session", "go test ./...", text),
		contextGuardPayload(t, "session", "grepple --json Widget .", text),
		contextGuardPayload(t, "session", "grepple Widget .", `{"schema":"grepple-results-v1"}`),
		contextGuardPayload(t, "session", "grepple Widget .", "grepple output spilled "+text),
	}
	for _, payload := range cases {
		if output := HandleContextGuard(payload); len(output) != 0 {
			t.Fatalf("unsupported output was changed: %s", output)
		}
	}
}

func TestContextGuardConcurrentUpdatesPreserveEntries(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	var wait sync.WaitGroup
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			output := strings.Repeat(string(rune('a'+index))+" source line\n", 20)
			HandleContextGuard(contextGuardPayload(t, "session", "grepple Widget .", output))
		}(index)
	}
	wait.Wait()
	path := filepath.Join(os.Getenv("GREPPLE_CONTEXT_GUARD_DIR"), "session.json")
	cache := readContextGuardCache(path, "session")
	if len(cache.Entries) != 12 {
		t.Fatalf("concurrent cache entries=%d want 12", len(cache.Entries))
	}
}

func contextGuardPayloadForTool(t *testing.T, sessionID, toolName string, toolInput map[string]any, output string) []byte {
	t.Helper()
	payload := map[string]any{
		"session_id": sessionID, "hook_event_name": "PostToolUse", "tool_name": toolName,
		"tool_input":    toolInput,
		"tool_response": map[string]any{"content": []map[string]any{{"type": "text", "text": output}}, "details": map[string]any{}},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func contextGuardPayload(t *testing.T, sessionID, command, output string) []byte {
	t.Helper()
	return contextGuardPayloadForTool(t, sessionID, "Bash", map[string]any{"command": command}, output)
}

func decodeUpdatedToolOutput(t *testing.T, output []byte) string {
	t.Helper()
	var decoded contextGuardHookOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Fatalf("hook event=%q", decoded.HookSpecificOutput.HookEventName)
	}
	return decoded.HookSpecificOutput.UpdatedToolOutput
}
