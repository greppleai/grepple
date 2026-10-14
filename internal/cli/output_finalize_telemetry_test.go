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
	for _, span := range spans {
		if span.Name == "cli.output_spill" && len(span.Attributes) != 0 {
			t.Fatal("ambiguous command errors must not be attributed to output finalization")
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
