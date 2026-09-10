package cli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

func TestNewResultRendererSelectsOutputMode(t *testing.T) {
	tests := []struct {
		name    string
		options cliOptions
		want    string
	}{
		{name: "files", options: cliOptions{Params: search.Params{Files: true}, JSON: "off"}, want: "filesRenderer"},
		{name: "count", options: cliOptions{Count: true, JSON: "off"}, want: "countRenderer"},
		{name: "json", options: cliOptions{JSON: "full"}, want: "jsonResultRenderer"},
		{name: "context", options: cliOptions{Params: search.Params{BeforeContext: 1}, JSON: "off"}, want: "contextRenderer"},
		{name: "only matching", options: cliOptions{OnlyMatching: true, JSON: "off"}, want: "onlyMatchingRenderer"},
		{name: "lines", options: cliOptions{LineOnly: true, JSON: "off"}, want: "lineRenderer"},
		{name: "segments", options: cliOptions{JSON: "off"}, want: "segmentRenderer"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			renderer := newResultRenderer(&test.options, newOutputWriter(&bytes.Buffer{}))
			if got := reflect.TypeOf(renderer).Name(); got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestLineRendererWritesToInjectedOutput(t *testing.T) {
	var output bytes.Buffer
	renderer := lineRenderer{output: newOutputWriter(&output), maxLines: 10}
	results := []api.FileResult{{
		Path:    "example.go",
		Matches: []api.ResultMatch{{Line: 7, Text: "needle"}},
	}}

	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "example.go:7:needle\n"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSegmentRendererPrintsRelatedGoPoints(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{{
		Path: "caller.go",
		Related: []api.RelatedSymbol{
			{
				Name: "service.Load → (*Store).Load", Path: "store.go", Kind: "method", Direction: "callee",
				Start: 12, End: 24, CallLine: 8, Confidence: "candidate",
				Segments: []api.ResultSegment{{Kind: "lines", Start: 12, End: 13, Text: "func (s *Store) Load() {\n}"}},
				Related:  []api.RelatedSymbol{{Name: "validate", Path: "validate.go", Direction: "callee", Start: 3, End: 7, CallLine: 13, Confidence: "unique"}},
			},
			{Name: "handle", Path: "handler.go", Kind: "func", Direction: "caller", Start: 30, End: 40, CallLine: 35, Confidence: "unique"},
		},
	}}

	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	want := "  → service.Load → (*Store).Load  store.go:12-24  call:8 [candidate; try --at store.go:12]\n"
	caller := "  ← handle  handler.go:30-40  call:35\n"
	preview := "    12   func (s *Store) Load() {\n    13   }\n    next:\n      → validate"
	if !strings.Contains(output.String(), "Next points (code navigation):\n") || !strings.Contains(output.String(), want) || !strings.Contains(output.String(), caller) || !strings.Contains(output.String(), preview) {
		t.Fatalf("related navigation missing from output:\n%s", output.String())
	}
}

func TestSegmentRendererReportsMatchesOmittedBySegmentLimit(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{{
		Path:     "example.go",
		Matches:  []api.ResultMatch{{Line: 3, Text: "needle"}, {Line: 40, Text: "needle"}},
		Segments: []api.ResultSegment{{Kind: "lines", Start: 1, End: 5, Text: "package example"}},
	}}

	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "1 matching lines omitted") || !strings.Contains(output.String(), "use --line-only") {
		t.Fatalf("missing omitted-match guidance:\n%s", output.String())
	}
}

func TestBoundedOutputWriterStopsBeforeAgentToolLimit(t *testing.T) {
	var output bytes.Buffer
	writer := newBoundedOutputWriter(&output, 256)
	err := writer.writeString(strings.Repeat("a", 500))
	if !errors.Is(err, errOutputTruncated) {
		t.Fatalf("write error = %v, want output truncation", err)
	}
	if output.Len() > 256 {
		t.Fatalf("bounded output wrote %d bytes, want at most 256", output.Len())
	}
	if !strings.Contains(output.String(), "grepple output truncated") {
		t.Fatalf("missing actionable truncation marker: %q", output.String())
	}
	if strings.Contains(output.String(), "aaaa") {
		t.Fatalf("truncation left a partial output line: %q", output.String())
	}
}
