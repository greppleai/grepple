package boundaries

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestRemoteCommandPreservesExactRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input api.AnalysisRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.Operation != api.AnalysisBoundaries || input.Repository != "owner/repo@tag~v1" {
			t.Fatalf("request=%+v", input)
		}
		result, _ := json.Marshal(map[string]any{"schema": "grepple-boundaries-v3"})
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	output := captureStdout(t, func() {
		if err := New(cliruntime.Environment{}).Run([]string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, `"repository": "owner/repo@tag~v1"`) {
		t.Fatalf("output=%s", output)
	}
}
