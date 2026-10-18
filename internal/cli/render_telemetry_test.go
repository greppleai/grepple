package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/internal/wire"
	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func assertOneRender(t *testing.T, spans []*spanpb.Span, format string, hasGraph bool) {
	t.Helper()
	assertCLIPhaseContract(t, spans)
	counts := map[string]int{}
	var render, command, navigation, local, scan *spanpb.Span
	for _, span := range spans {
		counts[span.Name]++
		switch span.Name {
		case "cli.render":
			render = span
		case "cli.command":
			command = span
		case "cli.navigation.execute":
			navigation = span
		case "cli.search.scan":
			scan = span
		case "cli.search.local":
			if spanAttributes(span)["cli.search.mode"].GetStringValue() == "outline" {
				local = span
			}
		}
	}
	if counts["cli.render"] != 1 || counts["cli.command"] != 1 || len(spans) > 9 || spanAttributes(render)["cli.render.format"].GetStringValue() != format {
		t.Fatalf("wrong/bounded render phase: %+v", counts)
	}
	parent := command
	if hasGraph {
		parent = navigation
	} else if local != nil {
		parent = local
	}
	if parent == nil || !bytes.Equal(render.ParentSpanId, parent.SpanId) || len(render.Events) != 0 {
		t.Fatalf("wrong render parent/events: %+v", counts)
	}
	if local != nil && (scan == nil || !bytes.Equal(scan.ParentSpanId, local.SpanId) || scan.StartTimeUnixNano < render.StartTimeUnixNano || scan.EndTimeUnixNano > render.EndTimeUnixNano) {
		t.Fatal("outline streaming scan must be nested within the actual render operation")
	}
}

func TestCLIRenderBranchesAndTruncatedOutput(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	path := filepath.Join(root, "private-render.go")
	if err := os.WriteFile(path, []byte("package renderfixture\nfunc Root(){ Target() }\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 26; i++ {
		if err := os.WriteFile(filepath.Join(root, "file-"+string(rune('a'+i))+".txt"), []byte("private-needle\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		args   []string
		format string
		graph  bool
		want   string
	}{
		{[]string{"search", "--json", "--limit", "0", "-F", "private-needle", root}, "json", false, "private-needle"},
		{[]string{"search", "--outline", path}, "text", false, "Target"},
		{[]string{"search", "--outline", "--json", path}, "json", false, `"files"`},
		{[]string{"search", "--count-by-repo", "-F", "private-needle", root}, "compact", false, "total"},
		{[]string{"search", "--count-by-repo", "--json", "-F", "private-needle", root}, "json", false, `"matches"`},
		{[]string{"graph", "callers", "--symbol", "Target", "--max-output-bytes", "160", path}, "compact", true, "grepple output truncated"},
		{[]string{"graph", "resolve", "--symbol", "Target", path}, "compact", true, "Target"},
	} {
		capture.reset()
		args := append([]string{"--no-repo-config"}, test.args...)
		output := captureStdout(t, func() {
			if err := Run(args); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(output, test.want) {
			t.Fatalf("%v output missing %q: %q", test.args, test.want, output)
		}
		spans, payload := capture.snapshot()
		assertOneRender(t, spans, test.format, test.graph)
		if bytes.Contains(payload, []byte(path)) || bytes.Contains(payload, []byte("private-needle")) || bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte("Target")) {
			t.Fatal("source, selector or path escaped in render telemetry")
		}
		wantGraphSpans := 7 // cold graph query reads actual sources
		if test.graph && test.args[1] == "resolve" {
			wantGraphSpans++ // cold resolve's observed universe and graph build
		}
		if test.graph && len(spans) != wantGraphSpans || !test.graph && test.args[1] == "--outline" && len(spans) != 7 {
			t.Fatalf("phase count varied with results or branch: %d", len(spans))
		}
	}

	// The same JSON output must be byte-for-byte unchanged when tracing is off.
	args := []string{"--no-repo-config", "search", "--json", "--limit", "0", "-F", "private-needle", root}
	on := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	graphArgs := []string{"--no-repo-config", "graph", "callers", "--symbol", "Target", "--json", path}
	graphOn := captureStdout(t, func() {
		if err := Run(graphArgs); err != nil {
			t.Fatal(err)
		}
	})
	body, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": false, "trace_endpoint": endpoint}})
	if err := os.WriteFile(settings, body, 0o600); err != nil {
		t.Fatal(err)
	}
	capture.reset()
	off := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	graphOff := captureStdout(t, func() {
		if err := Run(graphArgs); err != nil {
			t.Fatal(err)
		}
	})
	if on != off || graphOn != graphOff || capture.requestCount() != 0 {
		t.Fatal("search/graph output changed under telemetry or disabled invocation exported")
	}
}

func TestRemoteGraphRendersWithoutLocalNavigationPhase(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	graph := graphcommand.Output{Schema: graphcommand.Schema, Query: &graphcommand.Query{Direction: "callers", Depth: 1}}
	result, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/public/analysis" {
			t.Errorf("unexpected remote endpoint: %s", request.URL.Path)
		}
		_ = json.NewEncoder(response).Encode(wire.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: true, Complete: true, Result: result})
	}))
	defer backend.Close()
	for _, tc := range []struct {
		json   bool
		format string
	}{
		{false, "compact"}, {true, "json"},
	} {
		capture.reset()
		args := []string{"--no-repo-config", "graph", "callers", "--repo", "owner/repo", "--server", backend.URL, "--symbol", "SecretSelector"}
		if tc.json {
			args = append(args, "--json")
		}
		captureStdout(t, func() {
			if err := Run(args); err != nil {
				t.Fatal(err)
			}
		})
		spans, payload := capture.snapshot()
		assertOneRender(t, spans, tc.format, false)
		for _, span := range spans {
			if span.Name == "cli.navigation.execute" || span.Name == "cli.sources.resolve" || span.Name == "cli.navigation.cache_lookup" {
				t.Fatal("remote graph created a local navigation span")
			}
		}
		if bytes.Contains(payload, []byte("SecretSelector")) || bytes.Contains(payload, []byte("owner/repo")) || bytes.Contains(payload, []byte(backend.URL)) {
			t.Fatal("remote graph input appeared in render telemetry")
		}
	}
}
