package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/greppleai/grepple/api"
)

//revive:disable-next-line:cognitive-complexity
func TestRemoteAnalysisCommandsUseExactRepository(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var mu sync.Mutex
	var requests []api.AnalysisRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/public/analysis" {
			t.Errorf("path=%s", request.URL.Path)
		}
		var input api.AnalysisRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		requests = append(requests, input)
		mu.Unlock()
		result := remoteAnalysisTestResult(input.Operation)
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()

	commands := []struct {
		name string
		run  func() error
	}{
		{name: "graph", run: func() error {
			return runGraph([]string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"})
		}},
		{name: "architecture", run: func() error {
			return runArchitectureDirectory([]string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"})
		}},
		{name: "boundaries", run: func() error {
			return runBoundaries([]string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"})
		}},
		{name: "responsibilities", run: func() error {
			return runArchitectureResponsibilities([]string{"--json", "--server", server.URL, "--repo", "owner/repo@tag~v1"})
		}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				if err := command.run(); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(output, `"repository": "owner/repo@tag~v1"`) {
				t.Fatalf("output=%s", output)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("requests=%+v", requests)
	}
	want := []api.AnalysisOperation{api.AnalysisGraph, api.AnalysisArchitecture, api.AnalysisBoundaries, api.AnalysisResponsibilities}
	for index := range want {
		if requests[index].Operation != want[index] || requests[index].Repository != "owner/repo@tag~v1" {
			t.Fatalf("request[%d]=%+v", index, requests[index])
		}
	}
}

func TestAskGraphAndArchitectureSupportRemoteRepository(t *testing.T) {
	var operations []api.AnalysisOperation
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input api.AnalysisRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		operations = append(operations, input.Operation)
		result := remoteAnalysisTestResult(input.Operation)
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	session := newResearchSession(t.Context(), nil, t.TempDir(), server.URL)
	defer session.Close()
	if _, err := runAskGraphWithSession(session, t.TempDir(), askGraphInput{Direction: "impact", Symbol: "Run", Repository: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runAskArchitectureWithSession(session, t.TempDir(), askArchitectureInput{Operation: "directory", Repository: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || operations[0] != api.AnalysisGraph || operations[1] != api.AnalysisArchitecture {
		t.Fatalf("operations=%v", operations)
	}
}

func remoteAnalysisTestResult(operation api.AnalysisOperation) json.RawMessage {
	schema := map[api.AnalysisOperation]string{
		api.AnalysisGraph: "grepple-navigation-graph-v3", api.AnalysisArchitecture: "grepple-directory-architecture-v5",
		api.AnalysisBoundaries: "grepple-boundaries-v3", api.AnalysisResponsibilities: "grepple-directory-responsibilities-v2",
	}[operation]
	content, _ := json.Marshal(map[string]any{"schema": schema})
	return content
}

func TestValidateAnalysisResponseIdentityAndCompleteness(t *testing.T) {
	request := api.AnalysisRequest{Operation: api.AnalysisGraph, Repository: "owner/repo"}
	valid := api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: api.AnalysisGraph, Repository: "owner/repo", Found: true, Complete: false, Result: remoteAnalysisTestResult(api.AnalysisGraph)}
	if err := validateAnalysisResponse(request, valid); err != nil {
		t.Fatalf("explicitly incomplete response should remain usable: %v", err)
	}
	tests := []api.AnalysisResponse{
		{Schema: "wrong", Operation: api.AnalysisGraph, Repository: "owner/repo", Found: true, Result: valid.Result},
		{Schema: valid.Schema, Operation: api.AnalysisArchitecture, Repository: "owner/repo", Found: true, Result: valid.Result},
		{Schema: valid.Schema, Operation: api.AnalysisGraph, Repository: "other/repo", Found: true, Result: valid.Result},
		{Schema: valid.Schema, Operation: api.AnalysisGraph, Repository: "owner/repo", Found: false, Result: valid.Result},
		{Schema: valid.Schema, Operation: api.AnalysisGraph, Repository: "owner/repo", Found: true},
		{Schema: valid.Schema, Operation: api.AnalysisGraph, Repository: "owner/repo", Found: true, Result: remoteAnalysisTestResult(api.AnalysisArchitecture)},
	}
	for _, response := range tests {
		if err := validateAnalysisResponse(request, response); err == nil {
			t.Fatalf("invalid response succeeded: %+v", response)
		}
	}
}
