package architecture

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestRemoteCommandsPreserveExactRepository(t *testing.T) {
	var operations []api.AnalysisOperation
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input api.AnalysisRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.Repository != "owner/repo@tag~v1" {
			t.Fatalf("request=%+v", input)
		}
		operations = append(operations, input.Operation)
		schemas := map[api.AnalysisOperation]string{api.AnalysisArchitecture: "grepple-directory-architecture-v5", api.AnalysisResponsibilities: "grepple-directory-responsibilities-v2"}
		result, _ := json.Marshal(map[string]any{"schema": schemas[input.Operation]})
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	for _, args := range [][]string{{"directory", "--json"}, {"responsibilities", "--json"}} {
		var output bytes.Buffer
		args = append(args, "--server", server.URL, "--repo", "owner/repo@tag~v1")
		if err := New(cliruntime.Environment{Output: &output}).Run(args); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"repository": "owner/repo@tag~v1"`) {
			t.Fatalf("output=%s", output.String())
		}
	}
	if len(operations) != 2 || operations[0] != api.AnalysisArchitecture || operations[1] != api.AnalysisResponsibilities {
		t.Fatalf("operations=%v", operations)
	}
}
