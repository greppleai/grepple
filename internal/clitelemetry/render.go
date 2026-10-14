package clitelemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// RenderPhase measures one whole command-selected output call. It never wraps
// the writer, buffers the output, or observes rendered data. The caller chooses
// a literal format from the finite set below, outside any result iteration.
func RenderPhase(ctx context.Context, format string, output func() error) (err error) {
	phaseCtx, span := StartPhase(ctx, "cli.render")
	if span.IsRecording() {
		switch format {
		case "text", "json", "compact":
			span.SetAttributes(attribute.String("cli.render.format", format))
		}
	}
	defer func() { EndPhase(span, phaseCtx, err) }()
	return output()
}
