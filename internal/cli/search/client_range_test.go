package search

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/linerange"
)

func TestRemoteSearchPreservesFullLineRangeMiss(t *testing.T) {
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
}
