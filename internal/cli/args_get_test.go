package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/linerange"
)

func TestGetPreservesRemoteRangeClampWarningsAndMissStats(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := request.URL.Query().Get("start")
		if start == "29" {
			writer.Header().Set(linerange.HeaderOutcome, string(linerange.OutcomeFullMiss))
			writer.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			_, _ = writer.Write([]byte(`{"error":"requested line range 29-40 is outside file (1-28)"}`))
			return
		}
		writer.Header().Set(linerange.HeaderOutcome, string(linerange.OutcomePartialMiss))
		writer.Header().Set(linerange.HeaderWarning, "EOF: requested line range 20-40 exceeds file length 28; returned 20-28")
		_, _ = writer.Write([]byte("line 20\nline 21"))
	}))
	defer server.Close()

	var partialErr error
	warning := captureStderr(t, func() {
		output := captureStdout(t, func() {
			partialErr = Run([]string{"get", "--server", server.URL, "--lines", "20:40", "owner/repo", "sample.go"})
		})
		if output != "line 20\nline 21" {
			t.Fatalf("clamped output=%q", output)
		}
	})
	if partialErr != nil || !strings.Contains(warning, "returned 20-28") {
		t.Fatalf("partial err=%v warning=%q", partialErr, warning)
	}
	var fullErr error
	fullWarning := captureStderr(t, func() {
		fullErr = Run([]string{"get", "--server", server.URL, "--lines", "29:40", "owner/repo", "sample.go"})
	})
	if code, ok := ExitCode(fullErr); !ok || code != 1 || !strings.Contains(fullWarning, "outside file") {
		t.Fatalf("full miss error=%v warning=%q", fullErr, fullWarning)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Details.PartialLineRangeMisses != 1 || stats.Details.FullLineRangeMisses != 1 {
		t.Fatalf("range stats=%s", fmt.Sprintf("%#v", stats.Details))
	}
}
