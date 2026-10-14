package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

var cliPhaseAttributes = map[string][]string{
	"cli.invocation": {}, "cli.output_spill": {},
	"cli.command":                 {"cli.command.name"},
	"cli.search.local":            {"cli.search.mode", "cli.phase.outcome", "cli.results.count"},
	"cli.sources.discover":        {"cli.sources.mode", "cli.phase.outcome", "cli.sources.count"},
	"cli.sources.resolve":         {"cli.phase.outcome", "cli.sources.count"},
	"cli.navigation.execute":      {"cli.phase.outcome"},
	"cli.navigation.cache_lookup": {"cli.cache.kind", "cli.cache.state", "cli.phase.outcome"},
	"cli.render":                  {"cli.phase.outcome", "cli.render.format"},
}

var cliHTTPNames = []string{"cli.http.search", "cli.http.navigation", "cli.http.grit", "cli.http.analysis", "cli.http.raw", "cli.http.tree", "cli.http.repos", "cli.http.rules"}
var cliHTTPAttributes = []string{"http.request.method", "http.response.status_class", "grepple.outcome"}
var cliCommandCategories = []string{"search", "version", "write", "grit", "hook", "graph", "graph.resolve", "graph.callers", "graph.callees", "ask", "anchors", "examples", "artifacts", "architecture", "sources", "init", "verify", "languages", "metrics", "rules", "repos", "get", "tree", "refs", "ai-provider", "login", "logout"}

var cliAttributeValues = map[string][]string{
	"cli.command.name":           cliCommandCategories,
	"cli.search.mode":            {"content", "files", "at", "stdin", "count_summary"},
	"cli.sources.mode":           {"content", "listing"},
	"cli.cache.kind":             {"daemon"},
	"cli.cache.state":            {"hit", "miss"},
	"cli.render.format":          {"text", "json", "compact"},
	"cli.phase.outcome":          {"ok", "error", "cancelled"},
	"http.request.method":        {"GET", "POST", "DELETE", "other"},
	"http.response.status_class": {"1xx", "2xx", "3xx", "4xx", "5xx", "other"},
	"grepple.outcome":            {"canceled", "timeout", "network_error"},
}

func assertSafeAttribute(t *testing.T, attr *commonpb.KeyValue) {
	t.Helper()
	if attr.Key == "cli.sources.count" || attr.Key == "cli.results.count" {
		if _, ok := attr.Value.Value.(*commonpb.AnyValue_IntValue); !ok || attr.Value.GetIntValue() < 0 {
			t.Fatalf("invalid numeric count: %s", attr.Key)
		}
		return
	}
	allowed, ok := cliAttributeValues[attr.Key]
	if !ok || !slices.Contains(allowed, attr.Value.GetStringValue()) {
		t.Fatalf("unsafe attribute %s=%s", attr.Key, attr.Value.GetStringValue())
	}
}

func assertSafeSpan(t *testing.T, span *spanpb.Span) bool {
	t.Helper()
	if len(span.TraceId) != 16 || len(span.SpanId) != 8 || len(span.Events) != 0 || len(span.Links) != 0 {
		t.Fatalf("invalid/unbounded span data: %s", span.Name)
	}
	keys, known := cliPhaseAttributes[span.Name]
	http := strings.HasPrefix(span.Name, "cli.http.")
	if http {
		if !slices.Contains(cliHTTPNames, span.Name) || span.Kind != spanpb.Span_SPAN_KIND_CLIENT {
			t.Fatalf("unexpected HTTP CLIENT span: %s", span.Name)
		}
		keys, known = cliHTTPAttributes, true
	}
	if !known {
		t.Fatalf("unexpected span name: %q", span.Name)
	}
	for _, attr := range span.Attributes {
		if !slices.Contains(keys, attr.Key) {
			t.Fatalf("unexpected attribute on %s: %s", span.Name, attr.Key)
		}
		assertSafeAttribute(t, attr)
	}
	if span.Status != nil && !slices.Contains([]string{"", "error", "cancelled", "network_error", "timeout", "canceled", "http_error"}, span.Status.Message) {
		t.Fatalf("dynamic status on %s: %q", span.Name, span.Status.Message)
	}
	return http
}

func assertTraceRoot(t *testing.T, root, span *spanpb.Span) *spanpb.Span {
	t.Helper()
	if span.Name != "cli.invocation" {
		return root
	}
	if root != nil || len(span.ParentSpanId) != 0 {
		t.Fatal("duplicate/orphan root")
	}
	return span
}

// The output finalization span is a root child AFTER the command. It
// must never enclose the command's work or pretend all latency is spill time.
func assertInvocationWaterfall(t *testing.T, spans []*spanpb.Span, root *spanpb.Span) {
	t.Helper()
	var command, output *spanpb.Span
	for _, span := range spans {
		switch span.Name {
		case "cli.command":
			command = span
		case "cli.output_spill":
			output = span
		}
	}
	if command == nil || output == nil || !bytes.Equal(command.ParentSpanId, root.SpanId) || !bytes.Equal(output.ParentSpanId, root.SpanId) {
		t.Fatal("command/finalization must be sibling children of invocation")
	}
	if command.EndTimeUnixNano > output.StartTimeUnixNano || output.EndTimeUnixNano > root.EndTimeUnixNano {
		t.Fatal("finalization overlaps command or outlives invocation")
	}
}

func assertOneTraceTree(t *testing.T, spans []*spanpb.Span) {
	t.Helper()
	seen := make(map[string]*spanpb.Span, len(spans))
	var root *spanpb.Span
	for _, span := range spans {
		root = assertTraceRoot(t, root, span)
		if seen[string(span.SpanId)] != nil {
			t.Fatal("duplicate span ID")
		}
		seen[string(span.SpanId)] = span
	}
	if root == nil {
		t.Fatal("missing invocation root")
	}
	for _, span := range spans {
		if span == root {
			continue
		}
		parent := seen[string(span.ParentSpanId)]
		if parent == nil || !bytes.Equal(span.TraceId, root.TraceId) || !bytes.Equal(parent.TraceId, root.TraceId) {
			t.Fatalf("orphan/different trace span: %s", span.Name)
		}
	}
	assertInvocationWaterfall(t, spans, root)
}

// assertCLIPhaseContract is shared by cold/warm, small/large, local/remote,
// error and output-mode tests. HTTP CLIENT spans are per real request; only
// the newly added command phases are subject to the hard cardinality cap.
func assertCLIPhaseContract(t *testing.T, spans []*spanpb.Span) {
	t.Helper()
	phaseCount, httpCount := 0, 0
	for _, span := range spans {
		if assertSafeSpan(t, span) {
			httpCount++
		} else if !slices.Contains([]string{"cli.invocation", "cli.output_spill", "cli.command"}, span.Name) {
			phaseCount++
		}
	}
	if phaseCount > 10 || len(spans) != phaseCount+httpCount+3 {
		t.Fatalf("phase cap/baseline: phases=%d http=%d total=%d", phaseCount, httpCount, len(spans))
	}
	assertOneTraceTree(t, spans)
}
