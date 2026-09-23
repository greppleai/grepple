package search

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/apiclient"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/linerange"
)

func TestRemoteSearchPreservesAndCountsFullLineRangeMiss(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	t.Setenv("PI_SESSION_ID", "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set(linerange.HeaderOutcome, string(linerange.OutcomeFullMiss))
		writer.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		_, _ = writer.Write([]byte(`{"error":"requested line range 29-40 is outside file (1-28)"}`))
	}))
	defer server.Close()
	_, err := runTestRemote(&cliOptions{}, server.URL)
	if apiclient.RangeOutcome(err) != linerange.OutcomeFullMiss {
		t.Fatalf("remote range error=%T %v", err, err)
	}
	stats := rendercommand.ReadContextStatistics(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Details.FullLineRangeMisses != 1 {
		t.Fatalf("full range stats=%#v", stats.Details)
	}
}
