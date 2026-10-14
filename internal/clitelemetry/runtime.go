// Package clitelemetry owns opt-in, invocation-scoped CLI tracing.
package clitelemetry

import (
	"context"
	"net/http"
	"time"

	"github.com/greppleai/grepple/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Runtime is private to one CLI invocation. Nothing installs an ambient provider.
type Runtime struct {
	provider trace.TracerProvider
	sdk      *sdktrace.TracerProvider
}

// invocationKey distinguishes a user-enabled CLI trace from an unrelated
// incoming context that merely happens to contain a valid traceparent.
type invocationKey struct{}

// runtimeKey is installed only by an SDK-backed CLI invocation, not by
// WithInvocationTrace (which is also used to gate HTTP propagation).
type runtimeKey struct{}

// WithInvocationTrace explicitly opts a context into CLI backend propagation.
// The CLI calls this only when its user-owned telemetry setting is enabled.
func WithInvocationTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, invocationKey{}, true)
}

// Enabled reports whether this request belongs to an opted-in invocation.
func Enabled(ctx context.Context) bool {
	value, _ := ctx.Value(invocationKey{}).(bool)
	return value
}

// New creates a bounded OTLP/HTTP exporter only when tracing is explicitly enabled.
func New(settings *config.Telemetry) (*Runtime, error) {
	if settings == nil || !settings.Enabled {
		return &Runtime{provider: noop.NewTracerProvider()}, nil
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSHandshakeTimeout: time.Second, ResponseHeaderTimeout: time.Second}
	client := &http.Client{Transport: transport, Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(settings.TraceEndpoint), otlptracehttp.WithHTTPClient(client), otlptracehttp.WithHeaders(map[string]string{}), otlptracehttp.WithTimeout(time.Second), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
	if err != nil {
		return nil, err
	}
	return newWithExporter(*settings, exporter), nil
}

func newWithExporter(settings config.Telemetry, exporter sdktrace.SpanExporter) *Runtime {
	res := resource.NewWithAttributes("", attribute.String("service.name", "grepple-cli"))
	sdk := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(settings.SampleRatio()))), sdktrace.WithBatcher(exporter, sdktrace.WithMaxQueueSize(256), sdktrace.WithMaxExportBatchSize(64), sdktrace.WithBatchTimeout(200*time.Millisecond), sdktrace.WithExportTimeout(time.Second)))
	return &Runtime{provider: sdk, sdk: sdk}
}

// Start uses only fixed operation names, never user arguments or request URLs.
func (r *Runtime) Start(ctx context.Context, operation string) (context.Context, trace.Span) {
	switch operation {
	case "cli.invocation", "cli.command", "cli.output_spill", "cli.search.local", "cli.sources.discover", "cli.sources.resolve", "cli.navigation.execute", "cli.navigation.cache_lookup", "cli.navigation.build", "cli.navigation.query", "cli.render":
	default:
		operation = "cli.operation"
	}
	if r.sdk != nil && operation == "cli.invocation" {
		ctx = WithInvocationTrace(ctx)
		ctx = context.WithValue(ctx, runtimeKey{}, r)
	}
	return r.provider.Tracer("grepple.cli").Start(ctx, operation)
}

// Close gives a failed or slow collector a fixed budget and never changes CLI status.
func (r *Runtime) Close() {
	if r == nil || r.sdk == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.sdk.Shutdown(ctx)
}
