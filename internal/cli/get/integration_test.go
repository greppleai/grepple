package get

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/linerange"
)

func getTestApplication(output, diagnostics *bytes.Buffer, exit *int) cliruntime.Context {
	return cliruntime.Environment{
		Output:      output,
		ErrorOutput: diagnostics,
		Exit:        func(code int) { *exit = code },
		Config: cliruntime.ConfigurationServices{
			ContextGuard: func() bool { return true },
		},
	}
}

func TestRemoteRangeOutcomesUseCommandRenderingAndExit(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	t.Setenv("PI_SESSION_ID", "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("start") == "29" {
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

	var output, diagnostics bytes.Buffer
	exit := 0
	application := getTestApplication(&output, &diagnostics, &exit)
	if err := New(application).Run([]string{"--server", server.URL, "--lines", "20:40", "owner/repo", "sample.go"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "line 20\nline 21" || !strings.Contains(diagnostics.String(), "returned 20-28") {
		t.Fatalf("output=%q diagnostics=%q", output.String(), diagnostics.String())
	}
	output.Reset()
	diagnostics.Reset()
	if err := New(application).Run([]string{"--server", server.URL, "--lines", "29:40", "owner/repo", "sample.go"}); err != nil {
		t.Fatal(err)
	}
	if exit != 1 || !strings.Contains(diagnostics.String(), "outside file") {
		t.Fatalf("exit=%d diagnostics=%q", exit, diagnostics.String())
	}
	stats := rendercommand.ReadContextStatistics(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Details.PartialLineRangeMisses != 1 || stats.Details.FullLineRangeMisses != 1 {
		t.Fatalf("range stats=%s", fmt.Sprintf("%#v", stats.Details))
	}
}

func TestRemoteOutlineFetchesRawContentAndRendersLocally(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("format") == "json" {
			http.Error(writer, "should not request json", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("package p\n\ntype Widget struct{}\n\nfunc Build() {}\n"))
	}))
	defer server.Close()
	var output, diagnostics bytes.Buffer
	exit := 0
	if err := New(getTestApplication(&output, &diagnostics, &exit)).Run([]string{"--server", server.URL, "--outline", "owner/repo", "widget.go"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "struct\tWidget") || !strings.Contains(output.String(), "func\tBuild") {
		t.Fatalf("outline=%q", output.String())
	}
}
