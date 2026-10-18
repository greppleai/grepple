package clitelemetry

import (
	"github.com/greppleai/grepple/internal/workaccount"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SetWork reports actual request-owned local work on one whole-operation span.
// Call after all workers have joined; never report planned work as completed work.
func SetWork(span trace.Span, counts *workaccount.Counters) {
	if counts == nil || !span.IsRecording() {
		return
	}
	s := counts.Snapshot()
	attrs := []attribute.KeyValue{
		attribute.Int64("cli.files.attempted", s.Attempted),
		attribute.Int64("cli.files.read", s.Read),
		attribute.Int64("cli.files.read_failed", s.ReadFailed),
		attribute.Int64("cli.files.bytes_read", s.BytesRead),
	}
	if s.CandidatesObserved {
		attrs = append(attrs, attribute.Int64("cli.files.candidates", s.Candidates))
	}
	if s.SelectedObserved {
		attrs = append(attrs, attribute.Int64("cli.files.selected", s.Selected))
	}
	if s.ParseObserved {
		attrs = append(attrs,
			attribute.Int64("cli.files.parse_attempted", s.ParseAttempted),
			attribute.Int64("cli.files.parsed", s.Parsed),
			attribute.Int64("cli.files.parse_failed", s.ParseFailed),
		)
	}
	span.SetAttributes(attrs...)
}
