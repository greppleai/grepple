package clitelemetry

import (
	"context"
	"testing"

	"github.com/greppleai/grepple/internal/config"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInvocationScopedSpansAndShutdown(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime := newWithExporter(config.Telemetry{Enabled: true}, exporter)
	rootCtx, root := runtime.Start(context.Background(), "cli.invocation")
	commandCtx, command := runtime.Start(rootCtx, "cli.command")
	_, spill := runtime.Start(commandCtx, "secret-query-or-path")
	spill.End()
	command.End()
	root.End()
	if err := runtime.sdk.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("exported spans: %d", len(spans))
	}
	byName := make(map[string]bool)
	for _, span := range spans {
		byName[span.Name] = true
		if len(span.Attributes) != 0 || len(span.Events) != 0 {
			t.Fatalf("unexpected private attributes/events: %+v", span)
		}
		if span.Name != "cli.invocation" && span.Parent.TraceID() != root.SpanContext().TraceID() {
			t.Fatalf("child not in invocation trace: %+v", span)
		}
	}
	if !byName["cli.invocation"] || !byName["cli.command"] || !byName["cli.operation"] || byName["secret-query-or-path"] {
		t.Fatalf("span name allowlist: %+v", byName)
	}
}

func TestDisabledAndUnsampledHaveNoExports(t *testing.T) {
	if disabled, err := New(nil); err != nil {
		t.Fatal(err)
	} else {
		_, span := disabled.Start(context.Background(), "cli.invocation")
		if span.SpanContext().IsValid() {
			t.Fatal("disabled telemetry recorded a span")
		}
		span.End()
		disabled.Close()
	}
	zero := 0.0
	exporter := tracetest.NewInMemoryExporter()
	runtime := newWithExporter(config.Telemetry{Enabled: true, TraceSampleRatio: &zero}, exporter)
	_, span := runtime.Start(context.Background(), "cli.invocation")
	span.End()
	runtime.Close()
	if got := len(exporter.GetSpans()); got != 0 {
		t.Fatalf("unsampled spans: %d", got)
	}
}

func TestParallelInvocationsKeepIndependentTraceIDs(t *testing.T) {
	first, second := tracetest.NewInMemoryExporter(), tracetest.NewInMemoryExporter()
	one, two := newWithExporter(config.Telemetry{Enabled: true}, first), newWithExporter(config.Telemetry{Enabled: true}, second)
	_, a := one.Start(context.Background(), "cli.invocation")
	_, b := two.Start(context.Background(), "cli.invocation")
	if a.SpanContext().TraceID() == b.SpanContext().TraceID() {
		t.Fatal("parallel invocation traces collided")
	}
	a.End()
	b.End()
	if err := one.sdk.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := two.sdk.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	defer two.Close()
	if len(first.GetSpans()) != 1 || len(second.GetSpans()) != 1 {
		t.Fatal("invocation exporters shared state")
	}
}
