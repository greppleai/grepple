package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/archdaemon"
)

func TestGraphDaemonLocalOutputParityAndFlagRestrictions(t *testing.T) {
	chdirTemp(t)
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_CACHE_DIR", cache)
	t.Setenv("GREPPLE_NAVIGATION_CACHE_DIR", "")
	if err := os.WriteFile("main.go", []byte("package sample\nfunc Target() {}\nfunc Caller() { Target() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var result error
		output := captureStdout(t, func() { result = Run(append([]string{"--no-spill"}, args...)) })
		return output, result
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- archdaemon.Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	paths := []string{"main.go"}
	sources := analysis.ReadSources(paths)
	readyQuery := analysis.GraphQuery{Direction: "callers", Depth: 1, Symbol: "Target"}
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, ok := archdaemon.KeyGraph(paths, 0, sources, readyQuery); ok {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon did not become available")
	}
	for _, command := range [][]string{
		{"graph", "resolve", "--symbol", "Target", "--json", "main.go"},
		{"graph", "resolve", "--symbol", "Missing", "--json", "main.go"},
		{"graph", "callers", "--symbol", "Target", "--json", "main.go"},
		{"graph", "callees", "--symbol", "Target", "--json", "main.go"},
	} {
		name := strings.Join(command[1:4], "-")
		t.Run(name, func(t *testing.T) {
			direct, directErr := run(command...)
			cold, coldErr := run(append([]string{"--daemon"}, command...)...)
			warm, warmErr := run(append([]string{command[0], "--daemon"}, command[1:]...)...)
			if direct != cold || direct != warm || (directErr == nil) != (coldErr == nil) || (directErr == nil) != (warmErr == nil) {
				t.Fatalf("direct=%v cold=%v warm=%v\ndirect=%s\ncold=%s\nwarm=%s", directErr, coldErr, warmErr, direct, cold, warm)
			}
			if command[1] == "resolve" {
				if _, hit := archdaemon.QueryResolve(paths, 0, archdaemon.ResolveSelection{Symbol: command[3]}); !hit {
					t.Fatal("resolve report not published")
				}
			} else if _, hit := archdaemon.QueryGraph(paths, 0, analysis.GraphQuery{Direction: command[1], Depth: 1, Symbol: "Target"}); !hit {
				t.Fatal("focused graph report not published")
			}
		})
	}
	for _, command := range [][]string{{"--daemon", "tree", "."}} {
		if _, err := run(command...); err == nil || !strings.Contains(err.Error(), "--daemon") {
			t.Fatalf("unsupported daemon command %v returned %v", command, err)
		}
	}
}

func TestGraphDaemonRemoteQueryRemainsRemote(t *testing.T) {
	requests := make(chan wire.AnalysisRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
		var input wire.AnalysisRequest
		if err := json.NewDecoder(httpRequest.Body).Decode(&input); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- input
		_ = json.NewEncoder(writer).Encode(wire.AnalysisResponse{
			Schema: "grepple-remote-analysis-v1", Operation: wire.AnalysisGraph, Repository: input.Repository,
			Found: true, Complete: true, Result: json.RawMessage(`{"schema":"grepple-navigation-graph-v7"}`),
		})
	}))
	defer server.Close()
	var runError error
	output := captureStdout(t, func() {
		runError = Run([]string{"--no-spill", "--daemon", "graph", "callers", "--json", "--server", server.URL, "--repo", "owner/repo@main", "--symbol", "Target"})
	})
	if runError != nil {
		t.Fatal(runError)
	}
	select {
	case input := <-requests:
		if input.Operation != wire.AnalysisGraph || input.Repository != "owner/repo@main" || input.Graph == nil || input.Graph.Direction != "callers" {
			t.Fatalf("remote request=%+v", input)
		}
	default:
		t.Fatal("remote server was not contacted")
	}
	if !strings.Contains(output, `"repository": "owner/repo@main"`) {
		t.Fatalf("remote response=%s", output)
	}
}
