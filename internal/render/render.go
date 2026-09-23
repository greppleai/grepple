// Package render formats search and navigation results.
package render

import (
	"github.com/greppleai/grepple/navigation"
	"io"
	"strings"

	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

const anchorOutputSeparator = "│"

// AnchorLookup maps rendered file lines to editable anchors.
type AnchorLookup map[string]map[int]string

// Line returns an anchor for one file line.
func (lookup AnchorLookup) Line(path string, line int) string {
	if lines := lookup[path]; lines != nil {
		return lines[line]
	}
	return ""
}

// ContextGuard records and suppresses source already returned in the active context.
type ContextGuard interface {
	Seen(string, *navigation.ArtifactIdentity, search.ResultSegment) bool
	Record(string, *navigation.ArtifactIdentity, search.ResultSegment)
	RecordMarker(int)
	RecordSearchLine(string, int, string)
	RecordLineRangeOmission(int, int, int, int)
	LineCovered(string, int, string) bool
}

type noopContextGuard struct{}

func (noopContextGuard) Seen(string, *navigation.ArtifactIdentity, search.ResultSegment) bool {
	return false
}
func (noopContextGuard) Record(string, *navigation.ArtifactIdentity, search.ResultSegment) {}
func (noopContextGuard) RecordMarker(int)                                                  {}
func (noopContextGuard) RecordSearchLine(string, int, string)                              {}
func (noopContextGuard) RecordLineRangeOmission(int, int, int, int)                        {}
func (noopContextGuard) LineCovered(string, int, string) bool                              { return false }

// Options contains renderer-specific search options.
type Options struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	FilesWithMatches bool
	MaxOutputBytes   int
	RepeatSource     bool
	Stdin            bool
	Anchors          AnchorLookup
	Metadata         *api.ResultMetadata
}

type resultRenderer interface {
	Render([]search.FileResult) error
}

type outputWriter struct {
	writer io.Writer
	output *cliruntime.Output
}

func newOutputWriter(writer io.Writer, limits ...int) *outputWriter {
	maxBytes := 0
	if len(limits) > 0 {
		maxBytes = limits[0]
	}
	if maxBytes > 0 {
		return &outputWriter{writer: writer, output: cliruntime.NewBoundedOutput(writer, maxBytes)}
	}
	return &outputWriter{writer: writer, output: cliruntime.NewOutput(writer)}
}
func newBoundedOutputWriter(writer io.Writer, maxBytes int) *outputWriter {
	return newOutputWriter(writer, maxBytes)
}
func (output *outputWriter) writeString(value string) error { return output.output.WriteString(value) }
func (output *outputWriter) writeJSON(value any) error      { return output.output.WriteJSON(value) }
func (output *outputWriter) written() int                   { return output.output.Written() }

// Render formats results and returns the emitted byte count.
func Render(options Options, results []search.FileResult, destination io.Writer, guard ContextGuard) (int, error) {
	if guard == nil {
		guard = noopContextGuard{}
	}
	maxBytes := 0
	if options.JSON == "off" {
		maxBytes = options.MaxOutputBytes
	}
	output := newOutputWriter(destination, maxBytes)
	renderer := newResultRenderer(options, output, guard)
	err := renderer.Render(results)
	if err == cliruntime.ErrOutputTruncated {
		err = nil
	}
	return output.written(), err
}

func newResultRenderer(options Options, output *outputWriter, guard ContextGuard) resultRenderer {
	switch {
	case options.Params.Files || options.FilesWithMatches:
		return filesRenderer{output: output, json: options.JSON != "off"}
	case options.Count:
		return countRenderer{output: output, json: options.JSON != "off"}
	case options.JSON != "off":
		return jsonResultRenderer{output: output, matchesOnly: options.JSON == "matches" || options.LineOnly, metadata: options.Metadata, contextGuard: guard}
	case options.Params.BeforeContext > 0 || options.Params.AfterContext > 0:
		return contextRenderer{output: output, anchors: options.Anchors, contextGuard: guard}
	case options.OnlyMatching:
		return onlyMatchingRenderer{output: output, matcher: compileOnlyMatcher(&options)}
	case options.LineOnly:
		return lineRenderer{output: output, anchors: options.Anchors, contextGuard: guard, repeatSource: options.RepeatSource}
	default:
		return segmentRenderer{output: output, anchors: options.Anchors, contextGuard: guard}
	}
}

func filePathObjects(results []search.FileResult) []map[string]string {
	files := make([]map[string]string, 0, len(results))
	for _, result := range results {
		files = append(files, map[string]string{"path": result.Path})
	}
	return files
}
func normalizeRenderedAnchorLine(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}
