package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIOutputFinalizationFollowsCommandForArtifact(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, "artifacts"))
	file := filepath.Join(root, "private-fixture.txt")
	if err := os.WriteFile(file, []byte(strings.Repeat("private-query-text\n", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "search", "--json", "--spill-threshold-bytes", "128", "-F", "private-query-text", file}); err != nil {
			t.Fatal(err)
		}
	})
	var descriptor spilledOutputDescriptor
	if err := json.Unmarshal([]byte(output), &descriptor); err != nil || descriptor.Schema != "grepple-artifact-v1" {
		t.Fatalf("expected output artifact descriptor, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts", filepath.Base(descriptor.Path))); err != nil {
		t.Fatal(err)
	}
	spans, payload := capture.snapshot()
	assertOneRender(t, spans, "json", false)
	if bytes.Contains(payload, []byte("private-query-text")) || bytes.Contains(payload, []byte(file)) {
		t.Fatal("artifact input leaked to telemetry")
	}
	logical, err := os.ReadFile(filepath.Join(root, "artifacts", filepath.Base(descriptor.Path)))
	if err != nil || int64(len(logical)) != descriptor.Bytes {
		t.Fatalf("wrong persisted output size: %v", err)
	}
	for _, span := range spans {
		if span.Name != "cli.output_spill" {
			continue
		}
		attrs := spanAttributes(span)
		if attrs["cli.output.mode"].GetStringValue() != "spilled" || attrs["cli.output.logical_bytes"].GetIntValue() != int64(len(logical)) || attrs["cli.output.artifact_bytes"].GetIntValue() != int64(len(logical)) || attrs["cli.output.stdout_bytes"].GetIntValue() != int64(len(output)) {
			t.Fatalf("wrong artifact/stdout accounting: %+v", attrs)
		}
	}
}

func TestCLIOutputPreparationFailureDoesNotInventCommandOrFinalize(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	blocked := filepath.Join(t.TempDir(), "private-artifact-dir")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"--no-repo-config", "--artifact-dir", blocked, "version"}); err == nil {
		t.Fatal("expected output-spill preparation failure")
	}
	spans, payload := capture.snapshot()
	if len(spans) != 1 || spans[0].Name != "cli.invocation" || bytes.Contains(payload, []byte(blocked)) {
		t.Fatal("pre-command error invented a phase or exported its path")
	}
}

// The command and finalization are siblings; only finalization owns committed
// stdout bytes, including when the command itself returns a private error.
func TestCLIOutputAccountingInlineNoSpillAndError(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, "artifacts"))
	for _, test := range []struct {
		name      string
		args      []string
		wantError bool
	}{
		{name: "text", args: []string{"--no-repo-config", "version"}},
		{name: "json", args: []string{"--no-repo-config", "languages", "--json", "--spill-threshold-bytes", "1000000"}},
		{name: "no-spill", args: []string{"--no-repo-config", "languages", "--json", "--spill-threshold-bytes", "1", "--no-spill"}},
		{name: "command error", args: []string{"--no-repo-config", "graph", "resolve", "--symbol", "private-missing-symbol", root}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture.reset()
			output := captureStdout(t, func() {
				err := Run(test.args)
				if (err != nil) != test.wantError {
					t.Fatalf("unexpected command error state: %v", err)
				}
			})
			spans, payload := capture.snapshot()
			assertCLIPhaseContract(t, spans)
			if bytes.Contains(payload, []byte("private-missing-symbol")) || bytes.Contains(payload, []byte(root)) {
				t.Fatal("private input escaped in output metrics")
			}
			found := false
			for _, span := range spans {
				if span.Name != "cli.output_spill" {
					continue
				}
				found = true
				attrs := spanAttributes(span)
				if attrs["cli.output.mode"].GetStringValue() != "inline" || attrs["cli.output.logical_bytes"].GetIntValue() != int64(len(output)) || attrs["cli.output.stdout_bytes"].GetIntValue() != int64(len(output)) {
					t.Fatalf("inline metric mismatch: %+v output length %d", attrs, len(output))
				}
				if _, ok := attrs["cli.output.artifact_bytes"]; ok {
					t.Fatal("inline run invented artifact bytes")
				}
			}
			if !found {
				t.Fatal("missing finalization span")
			}
		})
	}
}

func TestCLIOutputAccountingSampledOutParity(t *testing.T) {
	capture, settings, _ := startPhaseCapture(t)
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, "artifacts"))
	args := []string{"--no-repo-config", "version"}
	on := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	spans, _ := capture.snapshot()
	assertCLIPhaseContract(t, spans)
	if err := os.WriteFile(settings, []byte(`{"telemetry":{"enabled":true,"trace_endpoint":"http://127.0.0.1:1/v1/traces","trace_sample_ratio":0}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	capture.reset()
	off := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	if on != off || capture.requestCount() != 0 {
		t.Fatal("sampled-out output changed or tracing exported")
	}
}
