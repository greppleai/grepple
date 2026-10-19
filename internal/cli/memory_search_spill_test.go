package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/memory"
)

func TestMemorySearchFooterSpillAndTelemetryPrivacy(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("file.go", []byte("package a\nfunc Run() { // needle\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, digest, err := memory.Source(root, "file.go")
	if err != nil {
		t.Fatal(err)
	}
	h := memory.Header{Schema: "grepple-memory-v1", Name: "private-note", Description: "Internal hint", Files: []memory.Attachment{{Path: "file.go", Checksum: digest}}}
	if err := memory.Save(root, h, []byte("Body must not appear implicitly"), false); err != nil {
		t.Fatal(err)
	}
	args := []string{"search", "--line-only", "-F", "needle", "file.go"}
	direct := captureStdout(t, func() {
		if err := Run(append(append([]string(nil), args...), "--no-spill")); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(direct, "grepple memory recall private-note") || strings.Contains(direct, "Body must not appear") {
		t.Fatalf("unsafe output %q", direct)
	}
	spilled := captureStdout(t, func() {
		if err := Run(append(append([]string(nil), args...), "--spill-threshold-bytes", "1", "--artifact-dir", "artifacts")); err != nil {
			t.Fatal(err)
		}
	})
	var artifactPath string
	for _, field := range strings.Fields(spilled) {
		if strings.HasPrefix(field, "path=") {
			artifactPath = strings.TrimPrefix(field, "path=")
			break
		}
	}
	if artifactPath == "" {
		t.Fatalf("missing artifact descriptor %q", spilled)
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifactPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != direct {
		t.Fatalf("spilled footer differs from direct output\n%s\n%s", content, direct)
	}
	_, payload := capture.snapshot()
	if bytes.Contains(payload, []byte("private-note")) || bytes.Contains(payload, []byte("Internal hint")) || bytes.Contains(payload, []byte("Body must not appear")) {
		t.Fatal("private memory appeared in telemetry")
	}
}
