package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type phaseCapture struct {
	mu       sync.Mutex
	spans    []*spanpb.Span
	payload  []byte
	requests int
}

func (capture *phaseCapture) reset() {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.spans = nil
	capture.payload = nil
	capture.requests = 0
}

func (capture *phaseCapture) snapshot() ([]*spanpb.Span, []byte) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]*spanpb.Span(nil), capture.spans...), append([]byte(nil), capture.payload...)
}

func (capture *phaseCapture) requestCount() int {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.requests
}

func startPhaseCapture(t *testing.T) (*phaseCapture, string, string) {
	t.Helper()
	capture := &phaseCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/traces" || request.Header.Get("Authorization") != "" {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			t.Error(err)
		}
		var exported tracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &exported); err != nil {
			t.Error(err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		capture.mu.Lock()
		capture.requests++
		capture.payload = append(capture.payload, body...)
		for _, resource := range exported.ResourceSpans {
			for _, scope := range resource.ScopeSpans {
				capture.spans = append(capture.spans, scope.Spans...)
			}
		}
		capture.mu.Unlock()
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	settings := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", settings)
	setEnabled := func(enabled bool, ratio float64) {
		body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": enabled, "trace_endpoint": server.URL + "/v1/traces", "trace_sample_ratio": ratio}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settings, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setEnabled(true, 1)
	return capture, settings, server.URL + "/v1/traces"
}

func spanAttributes(span *spanpb.Span) map[string]*commonpb.AnyValue {
	values := map[string]*commonpb.AnyValue{}
	for _, attribute := range span.Attributes {
		values[attribute.Key] = attribute.Value
	}
	return values
}

func assertSearchPhase(t *testing.T, spans []*spanpb.Span, command, mode string, expectedDiscovery, expectedRender bool) {
	t.Helper()
	assertCLIPhaseContract(t, spans)
	byName := map[string]*spanpb.Span{}
	counts := map[string]int{}
	for _, span := range spans {
		counts[span.Name]++
		byName[span.Name] = span
		if len(span.Events) != 0 {
			t.Fatalf("unexpected events: %s", span.Name)
		}
		// assertCLIPhaseContract above checks the exact span-specific attribute
		// allowlist, including the finite command option keys and value types.
	}
	wantScan := (mode == "content" || mode == "count_summary" || mode == "outline") && (expectedDiscovery || expectedRender)
	if counts["cli.invocation"] != 1 || counts["cli.output_spill"] != 1 || counts["cli.command"] != 1 || counts["cli.search.local"] != 1 || counts["cli.sources.discover"] > 1 || counts["cli.render"] > 1 || counts["cli.search.parse"] > 1 || counts["cli.search.scan"] > 1 || (counts["cli.search.scan"] == 1) != wantScan || len(spans) != 4+counts["cli.sources.discover"]+counts["cli.render"]+counts["cli.search.scan"]+counts["cli.search.parse"] || len(spans) > 8 {
		t.Fatalf("unbounded/absent phases: %+v", counts)
	}
	format := "text"
	if mode == "count_summary" {
		format = "compact"
	}
	if expectedRender != (counts["cli.render"] == 1) || expectedRender && (!bytes.Equal(byName["cli.render"].ParentSpanId, byName["cli.command"].SpanId) || spanAttributes(byName["cli.render"])["cli.render.format"].GetStringValue() != format) {
		t.Fatalf("wrong render phase: %+v", counts)
	}
	if expectedDiscovery != (counts["cli.sources.discover"] == 1) || spanAttributes(byName["cli.command"])["cli.command.name"].GetStringValue() != command || spanAttributes(byName["cli.search.local"])["cli.search.mode"].GetStringValue() != mode {
		t.Fatalf("wrong phases/labels: %+v", counts)
	}
	if !bytes.Equal(byName["cli.search.local"].ParentSpanId, byName["cli.command"].SpanId) || !bytes.Equal(byName["cli.command"].TraceId, byName["cli.invocation"].TraceId) {
		t.Fatal("local phase is not under command/invocation")
	}
	if expectedDiscovery && !bytes.Equal(byName["cli.sources.discover"].ParentSpanId, byName["cli.search.local"].SpanId) {
		t.Fatal("candidate collection is not under local search")
	}
	if counts["cli.search.scan"] == 1 && !bytes.Equal(byName["cli.search.scan"].ParentSpanId, byName["cli.search.local"].SpanId) || counts["cli.search.parse"] == 1 && !bytes.Equal(byName["cli.search.parse"].ParentSpanId, byName["cli.search.local"].SpanId) {
		t.Fatal("scan/parse batch is not under local search")
	}
	local := byName["cli.search.local"]
	for _, name := range []string{"cli.search.scan", "cli.search.parse"} {
		if stage := byName[name]; stage != nil && (stage.StartTimeUnixNano < local.StartTimeUnixNano || stage.EndTimeUnixNano > local.EndTimeUnixNano) {
			t.Fatalf("%s outside its operation wall-time boundary", name)
		}
	}
	if counts["cli.search.scan"] == 1 && counts["cli.sources.discover"] == 1 && byName["cli.sources.discover"].EndTimeUnixNano > byName["cli.search.scan"].StartTimeUnixNano {
		t.Fatal("initial discovery overlaps the subsequent scan")
	}
	if counts["cli.search.parse"] == 1 && byName["cli.search.scan"].EndTimeUnixNano > byName["cli.search.parse"].StartTimeUnixNano {
		t.Fatal("structural parse started before the primary scan completed")
	}
}

func TestCLIBoundedDiscoveryAndCommandCategory(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	secret := "private-query-not-for-export"
	for i := 0; i < 24; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("private-path-%02d.txt", i)), []byte(secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range []int{1, 24} {
		paths := make([]string, 0, size)
		for i := 0; i < size; i++ {
			paths = append(paths, filepath.Join(root, fmt.Sprintf("private-path-%02d.txt", i)))
		}
		args := append([]string{"--no-repo-config", "--no-spill", "search", "--no-related", "-F", "--line-only", "--limit", "0", secret}, paths...)
		if size == 1 {
			args = append([]string{"--no-repo-config", "--no-spill", "--no-related", "-F", "--line-only", "--limit", "0", secret}, paths...)
		}
		capture.reset()
		output := captureStdout(t, func() {
			if err := Run(args); err != nil {
				t.Fatal(err)
			}
		})
		if strings.Count(output, secret) != size {
			t.Fatalf("output does not match %d paths", size)
		}
		spans, payload := capture.snapshot()
		assertSearchPhase(t, spans, "search", "content", true, true)
		if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte("private-path-")) {
			t.Fatal("private source or query escaped to trace")
		}
	}
	capture.reset()
	output := captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "search", "--files", "--limit", "0", root}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Count(output, "private-path-") != 24 {
		t.Fatal("directory listing changed output")
	}
	spans, _ := capture.snapshot()
	assertSearchPhase(t, spans, "search", "files", true, true)

	// A recursive glob must also have just one candidate-discovery phase.
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "private-path-nested.txt"), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	capture.reset()
	output = captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "search", "--files", "--limit", "0", "**/*.txt"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "private-path-nested.txt") {
		t.Fatal("recursive glob missed nested file")
	}
	spans, _ = capture.snapshot()
	assertSearchPhase(t, spans, "search", "files", true, true)
	if err := os.Chdir(previous); err != nil {
		t.Fatal(err)
	}

	// A disabled or zero-sampled invocation must not contact the collector.
	for _, telemetry := range []struct {
		enabled bool
		ratio   float64
	}{{false, 1}, {true, 0}} {
		body, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": telemetry.enabled, "trace_endpoint": endpoint, "trace_sample_ratio": telemetry.ratio}})
		if err := os.WriteFile(settings, body, 0o600); err != nil {
			t.Fatal(err)
		}
		capture.reset()
		captureStdout(t, func() {
			if err := Run([]string{"--no-repo-config", "search", "--files", filepath.Join(root, "*.txt")}); err != nil {
				t.Fatal(err)
			}
		})
		if exported, _ := capture.snapshot(); len(exported) != 0 || capture.requestCount() != 0 {
			t.Fatal("disabled/unsampled invocation exported spans")
		}
	}
}

func TestCLIPhaseCountSummaryErrorAndRemoteOnly(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("unique-text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "search", "--count-summary", "-F", "unique-text", root}); err != nil {
			t.Fatal(err)
		}
	})
	spans, _ := capture.snapshot()
	assertSearchPhase(t, spans, "search", "count_summary", true, true)

	capture.reset()
	if err := Run([]string{"--no-repo-config", "search", "[", filepath.Join(root, "sample.txt")}); err == nil {
		t.Fatal("invalid pattern should fail")
	}
	spans, payload := capture.snapshot()
	assertSearchPhase(t, spans, "search", "content", false, false)
	for _, span := range spans {
		if span.Name == "cli.search.local" && spanAttributes(span)["cli.phase.outcome"].GetStringValue() != "error" {
			t.Fatal("failed matcher did not produce a fixed error outcome")
		}
	}
	if bytes.Contains(payload, []byte("sample.txt")) {
		t.Fatal("failed search exported filename")
	}

	backend := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/public/search/pages" {
			t.Errorf("unexpected remote path: %s", request.URL.Path)
		}
		fmt.Fprintf(response, `{"complete":true,"pageId":"%s","results":[{"path":"remote.txt","matches":[{"line":1,"text":"unique-text"}]}]}`, strings.Repeat("A", 43)+".0")
	}))
	defer backend.Close()
	capture.reset()
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "search", "--remote-only", "--server", backend.URL, "-F", "unique-text"}); err != nil {
			t.Fatal(err)
		}
	})
	spans, _ = capture.snapshot()
	for _, span := range spans {
		if span.Name == "cli.search.local" || span.Name == "cli.sources.discover" {
			t.Fatal("remote-only search created local phase")
		}
		if span.Name == "cli.command" && spanAttributes(span)["cli.command.name"].GetStringValue() != "search" {
			t.Fatal("remote-only command mislabeled")
		}
	}
	assertOneRender(t, spans, "text", false)
}
