package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
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
			renderer := newResultRenderer(&test.options, newOutputWriter(&bytes.Buffer{}), nil)
			if got := reflect.TypeOf(renderer).Name(); got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
		})
	}
}

func TestLineRendererWritesToInjectedOutput(t *testing.T) {
	var output bytes.Buffer
	renderer := lineRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{{
		Path: "example.go",
		Matches: []api.ResultMatch{
			{Line: 7, EndLine: 12, Text: "func needle() {"},
			{Line: 14, StartLine: 10, EndLine: 16, Text: "needle()"},
			{Line: 20, Text: "needle"},
		},
	}}

	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "example.go:7-12:func needle() {\nexample.go:14@10-16:needle()\nexample.go:20:needle\n"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSegmentRendererUsesWhitespaceForShortSpacingSegments(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{{
		Path: "example.go",
		Segments: []api.ResultSegment{
			{Kind: "summary", Start: 1, End: 1, Text: "first"},
			{Kind: "spacing", Start: 2, End: 3, Text: "\n"},
			{Kind: "lines", Start: 4, End: 4, Text: "fourth"},
		},
	}}
	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "example.go\n\n1   first\n\n\n4   fourth\n"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if strings.Contains(output.String(), "collapsed") {
		t.Fatalf("short whitespace gap used an omission marker: %q", output.String())
	}
}

func TestSegmentRendererPrintsRelatedGoPoints(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{{
		Path: "caller.go",
		Related: []api.RelatedSymbol{
			{Name: "Request", Path: "request.go", Kind: "struct", Direction: "type", Role: "parameter", Start: 2, End: 4, CallLine: 7, Confidence: "import-resolved", Segments: []api.ResultSegment{{Kind: "lines", Start: 2, End: 4, Text: "type Request struct {\n\tName string\n}"}}},
			{
				Name: "service.Load → (*Store).Load", Path: "store.go", Kind: "method", Direction: "callee",
				Start: 12, End: 24, CallLine: 8, Confidence: "candidate",
				Segments:       []api.ResultSegment{{Kind: "lines", Start: 12, End: 13, Text: "func (s *Store) Load() {\n}"}},
				Related:        []api.RelatedSymbol{{Name: "validate", Path: "validate.go", Direction: "callee", Start: 3, End: 7, CallLine: 13, Confidence: "unique"}},
				OmittedCallers: 2,
				OmittedCallees: 1,
				OmittedTypes:   1,
			},
			{Name: "handle", Path: "handler.go", Kind: "func", Direction: "caller", Start: 30, End: 40, CallLine: 35, Confidence: "unique"},
		},
		OmittedRelatedCallers: 3,
		OmittedRelatedCallees: 1,
		OmittedRelatedTypes:   2,
	}}

	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	want := "  → service.Load → (*Store).Load  store.go:12-24  call:8 [candidate; try --at store.go:12]\n"
	caller := "  ← handle  handler.go:30-40  call:35\n"
	typePoint := "  → Request  request.go:2-4  parameter-type:7\n"
	typeDefinition := "Related type definitions:\n\nrequest.go:2-4  Request\n2   type Request struct {"
	preview := "    12   func (s *Store) Load() {\n    13   }\n    next:\n      → validate"
	if !strings.Contains(output.String(), "Next points (code navigation):\n") || !strings.Contains(output.String(), want) || !strings.Contains(output.String(), caller) || !strings.Contains(output.String(), typePoint) || !strings.Contains(output.String(), typeDefinition) || !strings.Contains(output.String(), preview) {
		t.Fatalf("related navigation missing from output:\n%s", output.String())
	}
	for _, omission := range []string{
		"      … 1 additional callee and 2 additional callers omitted",
		"      … 1 additional type declaration omitted",
		"  … 1 additional callee and 3 additional callers omitted",
		"  … 2 additional type declarations omitted",
	} {
		if !strings.Contains(output.String(), omission) {
			t.Fatalf("related omission %q missing from output:\n%s", omission, output.String())
		}
	}
	for _, command := range []string{
		"grepple graph impact --at store.go:12 --depth 2 --json .",
		"grepple graph impact --at caller.go:1 --depth 2 --json .",
	} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("related output missing continuation %q:\n%s", command, output.String())
		}
	}
}

func TestSegmentRendererDeduplicatesAnchoredRelatedTypeAppendix(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{
		output:  newOutputWriter(&output),
		anchors: anchorLookup{"request.go": {2: "AAA", 3: "BBB", 4: "CCC"}},
	}
	typePoint := api.RelatedSymbol{
		Name: "Request", Path: "request.go", Kind: "struct", Direction: "type", Role: "parameter",
		Start: 2, End: 4, CallLine: 7, Confidence: "import-resolved",
		Segments: []api.ResultSegment{{Kind: "lines", Start: 2, End: 4, Text: "type Request struct {\n\tName string\n}"}},
	}
	results := []api.FileResult{
		{Path: "first.go", Related: []api.RelatedSymbol{typePoint}},
		{Path: "second.go", Related: []api.RelatedSymbol{typePoint}},
	}
	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	if strings.Count(rendered, "type Request struct {") != 1 {
		t.Fatalf("related type definition was not deduplicated:\n%s", rendered)
	}
	if !strings.Contains(rendered, "AAA│2│type Request struct {") || strings.LastIndex(rendered, "Related type definitions:") < strings.LastIndex(rendered, "second.go") {
		t.Fatalf("related type appendix is not anchored at the end:\n%s", rendered)
	}
}

func TestSegmentRendererShowsResolvedExternalTypeDefinition(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	artifact := &api.NavigationArtifactIdentity{
		Ecosystem: "go", Module: "github.com/gofiber/fiber/v3", Version: "v3.5.0",
		Repository: "gofiber/fiber@tag~v3.5.0", Commit: "abcdef", Digest: "artifact-digest",
	}
	results := []api.FileResult{{
		Path: "internal/shard/health_controller.go",
		Related: []api.RelatedSymbol{{
			Name: "Ctx", Path: "ctx.go", Kind: "interface", Direction: "type", Role: "parameter",
			Start: 17, End: 20, CallLine: 15, Confidence: "dependency-resolved", Artifact: artifact,
			Segments: []api.ResultSegment{{Kind: "lines", Start: 17, End: 20, Text: "type Ctx interface {\n\tRequest() *Request\n\tResponse() *Response\n}"}},
		}},
	}}
	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"→ Ctx  gofiber/fiber@tag~v3.5.0:ctx.go:17-20  parameter-type:15",
		"Related type definitions:",
		"gofiber/fiber@tag~v3.5.0:ctx.go:17-20  Ctx [github.com/gofiber/fiber/v3@v3.5.0; commit abcdef]",
		"type Ctx interface {",
		"Request() *Request",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("resolved external type output missing %q:\n%s", expected, rendered)
		}
	}
}

func TestSegmentRendererReportsIncompleteSourceAnalysis(t *testing.T) {
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output)}
	results := []api.FileResult{
		{Path: "valid.go", StructureStatus: string(parser.SegmentBuildStructured)},
		{Path: "recovered.go", StructureStatus: string(parser.SegmentBuildRecovered)},
		{Path: "plain.txt", StructureStatus: string(parser.SegmentBuildPlain)},
		{Path: "unknown.ext", StructureStatus: string(parser.SegmentBuildUnsupported)},
		{Path: "invalid.go", StructureStatus: string(parser.SegmentBuildFailed)},
	}
	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	want := "! incomplete source analysis returned=5 structured=1 plain=1 unsupported=1 failed=1 recovered=1"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("missing source analysis %q:\n%s", want, output.String())
	}
	analysis := searchSourceAnalysis(results)
	if analysis == nil || analysis.Returned != 5 || analysis.Recovered != 1 || analysis.Failed != 1 {
		t.Fatalf("analysis=%#v", analysis)
	}

	output.Reset()
	jsonRenderer := jsonResultRenderer{output: newOutputWriter(&output)}
	if err := jsonRenderer.Render(results); err != nil {
		t.Fatal(err)
	}
	var response api.SearchResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.SourceAnalysis == nil || *response.SourceAnalysis != *analysis {
		t.Fatalf("JSON source analysis=%#v, want %#v", response.SourceAnalysis, analysis)
	}
}

func TestSearchJSONIncludesStandardPagingMetadata(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "a.txt", "needle\n")
	writeGraphSource(t, dir, "b.txt", "needle\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"search", "--json", "--limit", "1", "needle", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var response api.SearchResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	metadata := response.Metadata
	if metadata == nil || metadata.Page.Returned != 1 || metadata.Page.Limit != 1 || metadata.Page.Complete || metadata.NextCommand == "" {
		t.Fatalf("search metadata=%#v", metadata)
	}
	if !strings.Contains(metadata.NextCommand, "grepple search --skip 1 --limit 1 --json") {
		t.Fatalf("search continuation is not copyable: %q", metadata.NextCommand)
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

func TestAnchoredContextRendererEmitsEditableRows(t *testing.T) {
	var output bytes.Buffer
	renderer := contextRenderer{output: newOutputWriter(&output), anchors: anchorLookup{"sample.go": {1: "AAA", 2: "BBB", 3: "CCC"}}}
	results := []api.FileResult{{Path: "sample.go", Context: []api.ContextLine{{Line: 1, Text: "before"}, {Line: 2, Text: "needle", Match: true}, {Line: 3, Text: "after"}}}}
	if err := renderer.Render(results); err != nil {
		t.Fatal(err)
	}
	expected := "sample.go\n\nAAA│1│before\nBBB│2│needle\nCCC│3│after\n"
	if output.String() != expected {
		t.Fatalf("output=%q expected=%q", output.String(), expected)
	}
}
