package clitelemetry

import (
	"context"
	"testing"

	"github.com/greppleai/grepple/internal/config"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestPhaseRequiresOwnedRecordingInvocationAndFixedName(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	runtime := newWithExporter(config.Telemetry{Enabled: true}, exporter)
	rootCtx, root := runtime.Start(context.Background(), "cli.invocation")
	commandCtx, command := runtime.Start(rootCtx, "cli.command")

	foreign := sdktrace.NewTracerProvider()
	defer foreign.Shutdown(context.Background())
	foreignCtx, foreignSpan := foreign.Tracer("foreign").Start(context.Background(), "foreign")
	defer foreignSpan.End()
	for _, ctx := range []context.Context{context.Background(), WithInvocationTrace(context.Background()), WithInvocationTrace(foreignCtx), context.WithValue(commandCtx, runtimeKey{}, (*Runtime)(nil))} {
		_, phase := StartPhase(ctx, "cli.search.local")
		if phase.IsRecording() {
			t.Fatal("unowned context recorded a phase")
		}
		EndPhase(phase, ctx, nil)
	}
	_, rejected := StartPhase(commandCtx, "cli.secret-source")
	if rejected.IsRecording() {
		t.Fatal("arbitrary phase name was accepted")
	}
	EndPhase(rejected, commandCtx, nil)
	_, phase := StartPhase(commandCtx, "cli.search.local")
	if !phase.IsRecording() || phase.SpanContext().TraceID() != root.SpanContext().TraceID() {
		t.Fatal("recording phase has incorrect parent")
	}
	EndPhase(phase, commandCtx, context.Canceled)
	command.End()
	root.End()
	if err := runtime.sdk.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	spans := exporter.GetSpans()
	if len(spans) != 3 || spans[0].Name != "cli.search.local" || spans[0].Parent.SpanID() != command.SpanContext().SpanID() || string(spans[0].Status.Description) != "cancelled" {
		t.Fatalf("unexpected phase export: %+v", spans)
	}
}

func TestUnsampledPhaseDoesNotRecord(t *testing.T) {
	zero := 0.0
	exporter := tracetest.NewInMemoryExporter()
	runtime := newWithExporter(config.Telemetry{Enabled: true, TraceSampleRatio: &zero}, exporter)
	rootCtx, root := runtime.Start(context.Background(), "cli.invocation")
	_, phase := StartPhase(rootCtx, "cli.search.local")
	if phase.IsRecording() {
		t.Fatal("unsampled phase recorded")
	}
	EndPhase(phase, rootCtx, nil)
	root.End()
	runtime.Close()
	if len(exporter.GetSpans()) != 0 {
		t.Fatal("unsampled phase exported")
	}
}

// BenchmarkDisabledPhase compares one additional no-op phase boundary with a
// baseline no-op span operation. Both paths have no exporter or network client.
func BenchmarkDisabledPhase(b *testing.B) {
	ctx := context.Background()
	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			trace.SpanFromContext(ctx).End()
		}
	})
	b.Run("disabled_phase", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			phaseCtx, span := StartPhase(ctx, "cli.search.local")
			EndPhase(span, phaseCtx, nil)
		}
	})
}
