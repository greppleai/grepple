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
	"cli.invocation": {}, "cli.output_spill": {"cli.output.mode", "cli.output.logical_bytes", "cli.output.stdout_bytes", "cli.output.artifact_bytes"},
	"cli.command":                 {"cli.command.name", "cli.command.source_mode", "cli.command.output_format", "cli.command.option.no_spill", "cli.command.option.no_repo_config", "cli.command.option.no_config_ignore", "cli.command.option.production_only", "cli.command.option.daemon_requested", "cli.command.option.at", "cli.command.option.outline", "cli.command.option.files", "cli.command.option.files_with_matches", "cli.command.option.count_summary", "cli.command.option.related_requested", "cli.command.option.no_related_requested", "cli.command.option.line_only", "cli.command.option.fixed_strings", "cli.command.option.ignore_case", "cli.command.option.json_matches", "cli.command.option.cursor_requested", "cli.command.option.metadata_filter_requested"},
	"cli.search.local":            {"cli.search.mode", "cli.phase.outcome", "cli.results.count", "cli.files.candidates", "cli.files.selected", "cli.files.attempted", "cli.files.read", "cli.files.read_failed", "cli.files.bytes_read", "cli.files.parse_attempted", "cli.files.parsed", "cli.files.parse_failed"},
	"cli.search.scan":             {"cli.phase.outcome"},
	"cli.search.parse":            {"cli.phase.outcome"},
	"cli.sources.discover":        {"cli.sources.mode", "cli.phase.outcome", "cli.sources.count"},
	"cli.sources.resolve":         {"cli.phase.outcome", "cli.sources.count"},
	"cli.navigation.execute":      {"cli.phase.outcome", "cli.files.attempted", "cli.files.read", "cli.files.read_failed", "cli.files.bytes_read", "cli.files.parse_attempted", "cli.files.parsed", "cli.files.parse_failed"},
	"cli.navigation.cache_lookup": {"cli.cache.kind", "cli.cache.state", "cli.phase.outcome"},
	"cli.navigation.read":         {"cli.phase.outcome"},
	"cli.navigation.build":        {"cli.phase.outcome"},
	"cli.render":                  {"cli.phase.outcome", "cli.render.format"},
}

var cliHTTPNames = []string{"cli.http.search", "cli.http.navigation", "cli.http.grit", "cli.http.analysis", "cli.http.raw", "cli.http.tree", "cli.http.repos", "cli.http.rules"}
var cliHTTPAttributes = []string{"http.request.method", "http.response.status_class", "grepple.outcome"}
var cliCommandCategories = []string{"search", "version", "write", "grit", "hook", "graph", "graph.resolve", "graph.callers", "graph.callees", "ask", "anchors", "examples", "artifacts", "architecture", "sources", "init", "verify", "overview", "languages", "metrics", "rules", "repos", "get", "tree", "refs", "ai-provider", "login", "logout"}

var cliAttributeValues = map[string][]string{
	"cli.command.name":           cliCommandCategories,
	"cli.command.source_mode":    {"local", "mixed", "remote"},
	"cli.command.output_format":  {"text", "json", "compact"},
	"cli.search.mode":            {"content", "files", "at", "stdin", "count_summary", "outline"},
	"cli.sources.mode":           {"content", "listing"},
	"cli.cache.kind":             {"daemon"},
	"cli.cache.state":            {"hit", "miss"},
	"cli.render.format":          {"text", "json", "compact"},
	"cli.output.mode":            {"inline", "spilled"},
	"cli.phase.outcome":          {"ok", "error", "cancelled"},
	"http.request.method":        {"GET", "POST", "DELETE", "other"},
	"http.response.status_class": {"1xx", "2xx", "3xx", "4xx", "5xx", "other"},
	"grepple.outcome":            {"canceled", "timeout", "network_error"},
}

func assertSafeAttribute(t *testing.T, attr *commonpb.KeyValue) {
	t.Helper()
	if strings.HasPrefix(attr.Key, "cli.command.option.") {
		if !slices.Contains(cliPhaseAttributes["cli.command"], attr.Key) || !attr.Value.GetBoolValue() {
			t.Fatalf("invalid command option %s=%v", attr.Key, attr.Value)
		}
		if _, ok := attr.Value.Value.(*commonpb.AnyValue_BoolValue); !ok {
			t.Fatalf("command option is not a Boolean: %s", attr.Key)
		}
		return
	}
	if attr.Key == "cli.sources.count" || attr.Key == "cli.results.count" || strings.HasPrefix(attr.Key, "cli.files.") || strings.HasPrefix(attr.Key, "cli.output.") && attr.Key != "cli.output.mode" {
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
	if span.Name == "cli.command" && len(span.Attributes) > 20 {
		t.Fatalf("command attribute budget exceeded: %d", len(span.Attributes))
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
