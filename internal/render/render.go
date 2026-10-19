// Package render formats search and navigation results.
package render

import (
	"io"
	"strings"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
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

// Options contains renderer-specific search options.
type Options struct {
	Params           search.Params
	LineOnly         bool
	OnlyMatching     bool
	JSON             string
	Count            bool
	FilesWithMatches bool
	MaxOutputBytes   int
	Stdin            bool
	Anchors          AnchorLookup
	Metadata         *wire.ResultMetadata
	Memories         *memory.Discovery
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
func (output *outputWriter) canAppend(size int) bool        { return output.output.CanAppend(size) }

// Render formats results and returns the emitted byte count.
func Render(options Options, results []search.FileResult, destination io.Writer) (int, error) {
	written, _, err := renderWithMemory(options, results, destination)
	return written, err
}

// renderWithMemory writes a footer through the SAME bounded output pipeline as
// source results. Its flag signals only a footer suppressed by the byte cap.
func renderWithMemory(options Options, results []search.FileResult, destination io.Writer) (int, bool, error) {
	maxBytes := 0
	if options.JSON == "off" {
		maxBytes = options.MaxOutputBytes
	}
	output := newOutputWriter(destination, maxBytes)
	renderer := newResultRenderer(options, output)
	err := renderer.Render(results)
	if err == cliruntime.ErrOutputTruncated {
		return output.written(), false, nil
	}
	if err != nil {
		return output.written(), false, err
	}
	if options.JSON == "off" && options.Memories != nil {
		footer := memoryFooter(*options.Memories)
		if footer != "" {
			if !output.canAppend(len(footer)) {
				return output.written(), true, nil
			}
			if err = output.writeString(footer); err != nil {
				return output.written(), false, err
			}
		}
	}
	return output.written(), false, nil
}

func newResultRenderer(options Options, output *outputWriter) resultRenderer {
	switch {
	case options.Params.Files || options.FilesWithMatches:
		return filesRenderer{output: output, json: options.JSON != "off"}
	case options.Count:
		return countRenderer{output: output, json: options.JSON != "off"}
	case options.JSON != "off":
		return jsonResultRenderer{output: output, matchesOnly: options.JSON == "matches" || options.LineOnly, metadata: options.Metadata, annotateScopes: options.Params.AnnotateScopes, memories: options.Memories}
	case options.Params.BeforeContext > 0 || options.Params.AfterContext > 0:
		return contextRenderer{output: output, anchors: options.Anchors}
	case options.OnlyMatching:
		return onlyMatchingRenderer{output: output, matcher: compileOnlyMatcher(&options)}
	case options.LineOnly:
		return lineRenderer{output: output, anchors: options.Anchors, annotateScopes: options.Params.AnnotateScopes}
	default:
		return segmentRenderer{output: output, anchors: options.Anchors, annotateScopes: options.Params.AnnotateScopes}
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
