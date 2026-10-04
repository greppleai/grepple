package pihooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHandleLintHookIgnoresIrrelevantInput(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{
		[]byte("not json"),
		[]byte(`{}`),
		[]byte(`{"hook_event_name":"PreToolUse","cwd":"."}`),
		[]byte(`{"hook_event_name":"Stop","cwd":""}`),
		[]byte(`{"hook_event_name":"Stop","cwd":"/does/not/exist"}`),
	} {
		if output := HandleLintHook(input, "."); len(output) != 0 {
			t.Errorf("HandleLintHook(%q) = %q, want empty output", input, output)
		}
	}
}

func TestMarshalStopFeedbackProtocol(t *testing.T) {
	t.Parallel()
	progress := 4
	output := marshalStopFeedback("fix this", &progress)
	var decoded struct {
		HookSpecificOutput struct {
			HookEventName        string `json:"hookEventName"`
			AdditionalContext    string `json:"additionalContext"`
			ContinuationProgress int    `json:"continuationProgress"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.HookSpecificOutput
	if got.HookEventName != "Stop" || got.AdditionalContext != "fix this" || got.ContinuationProgress != progress {
		t.Fatalf("unexpected output: %+v", got)
	}
}

func TestTruncateUTF8(t *testing.T) {
	t.Parallel()
	input := strings.Repeat("a", maxFeedbackBytes-1) + "界tail"
	got := truncateUTF8(input, maxFeedbackBytes)
	if len(got) > maxFeedbackBytes || !utf8.ValidString(got) {
		t.Fatalf("invalid truncation: bytes=%d valid=%v", len(got), utf8.ValidString(got))
	}
}

func TestNonEmptyLines(t *testing.T) {
	t.Parallel()
	got := nonEmptyLines(" a \n\n b\r\n")
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("nonEmptyLines = %#v", got)
	}
}

func TestGoPackagesReportsGoListFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(root+"/go.mod", []byte("not a go module"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := goPackages(root); err == nil || !strings.Contains(err.Error(), "go list ./... failed") {
		t.Fatalf("goPackages error = %v", err)
	}
}

func TestStopFormattingCheckNeverModifiesSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := []byte("package fixture\n//grepple: entity\ntype X struct{Value string}\n")
	path := filepath.Join(root, "fixture.go")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	event, _ := json.Marshal(map[string]string{"hook_event_name": "Stop", "cwd": root})
	output := HandleLintHook(event, ".")
	if !strings.Contains(string(output), "require formatting") || !strings.Contains(string(output), "no source files were changed") {
		t.Fatalf("missing read-only feedback: %s", output)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(source, after) {
		t.Fatalf("verifier changed source: %q, %v", after, err)
	}
}
