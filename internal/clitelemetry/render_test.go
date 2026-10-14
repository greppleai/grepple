package clitelemetry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/greppleai/grepple/internal/config"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRenderPhasePreservesWriterAndFixedOutcome(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime := newWithExporter(config.Telemetry{Enabled: true}, exporter)
	rootCtx, root := runtime.Start(context.Background(), "cli.invocation")
	commandCtx, command := runtime.Start(rootCtx, "cli.command")
	var actual bytes.Buffer
	private := []byte("secret-path:private-source")
	failure := io.ErrShortWrite
	if err := RenderPhase(commandCtx, "compact", func() error {
		_, _ = actual.Write(private)
		return failure
	}); !errors.Is(err, failure) {
		t.Fatalf("changed writer error: %v", err)
	}
	if !bytes.Equal(actual.Bytes(), private) {
		t.Fatalf("changed output bytes: %q", actual.Bytes())
	}
	command.End()
	root.End()
	if err := runtime.sdk.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	spans := exporter.GetSpans()
	if len(spans) != 3 || spans[0].Name != "cli.render" || spans[0].Parent.SpanID() != command.SpanContext().SpanID() || spans[0].Status.Description != "error" {
		t.Fatalf("incorrect render span/parent/error outcome: %+v", spans)
	}
	if len(spans[0].Attributes) != 2 {
		t.Fatalf("unexpected render attributes: %+v", spans[0].Attributes)
	}
	for _, attr := range spans[0].Attributes {
		if attr.Key == "cli.render.format" && attr.Value.AsString() != "compact" || attr.Key == "cli.phase.outcome" && attr.Value.AsString() != "error" || attr.Key != "cli.render.format" && attr.Key != "cli.phase.outcome" {
			t.Fatalf("unsafe render attribute: %v", attr)
		}
	}
}
