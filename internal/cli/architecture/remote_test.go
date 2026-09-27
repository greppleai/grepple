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

func TestRemoteDirectoryPreservesExactRepository(t *testing.T) {
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
		result, _ := json.Marshal(map[string]any{"schema": "grepple-directory-architecture-v5"})
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	var output bytes.Buffer
	command := New(cliruntime.Environment{Output: &output}).(*command)
	if err := command.Run([]string{"directory", "--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"repository": "owner/repo@tag~v1"`) {
		t.Fatalf("output=%s", output.String())
	}
	if len(operations) != 1 || operations[0] != api.AnalysisArchitecture {
		t.Fatalf("operations=%v", operations)
	}
}
