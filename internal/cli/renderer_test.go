package cli

import (
	"bytes"
	"reflect"
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
