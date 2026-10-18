package clitelemetry

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartPhase starts one of the fixed CLI phases only under this invocation's
// private, recording span. An unrelated traceparent or the public propagation
// marker alone cannot create a CLI phase.
func StartPhase(ctx context.Context, name string) (context.Context, trace.Span) {
	noop := trace.SpanFromContext(context.Background())
	if ctx == nil || !Enabled(ctx) {
		return ctx, noop
	}
	runtime, _ := ctx.Value(runtimeKey{}).(*Runtime)
	parent := trace.SpanFromContext(ctx)
	if runtime == nil || runtime.sdk == nil || !parent.IsRecording() || !parent.SpanContext().IsValid() {
		return ctx, noop
	}
	switch name {
	case "cli.search.local", "cli.search.scan", "cli.search.parse", "cli.sources.discover", "cli.sources.resolve", "cli.navigation.execute", "cli.navigation.cache_lookup", "cli.navigation.read", "cli.navigation.build", "cli.navigation.query", "cli.render":
		return runtime.Start(ctx, name)
	default:
		return ctx, noop
	}
}

// EndPhase records only a fixed outcome; raw error messages are never exported.
func EndPhase(span trace.Span, ctx context.Context, err error) {
	if span.IsRecording() {
		outcome := "ok"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx != nil && ctx.Err() != nil {
			outcome = "cancelled"
		} else if err != nil {
			outcome = "error"
		}
		span.SetAttributes(attribute.String("cli.phase.outcome", outcome))
		if outcome != "ok" {
			span.SetStatus(codes.Error, outcome)
		}
	}
	span.End()
}
