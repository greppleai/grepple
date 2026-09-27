package render

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
)

// OutlineOptions controls outline discovery and output.
type OutlineOptions struct {
	Params         search.Params
	Depth          int
	Kinds          OutlineKinds
	JSON           bool
	MaxOutputBytes int
	DefaultLimit   int
	Output         io.Writer
	ErrorOutput    io.Writer
}

// Outlines loads and renders structural file outlines and reports whether human
// output contained a file.
func Outlines(options OutlineOptions) (bool, error) {
	paths, err := search.ListFilePaths(options.Params, nil)
	if err != nil {
		return false, err
	}
	if options.Params.Limit == options.DefaultLimit && options.Params.MaxFiles == 0 && len(paths) == options.DefaultLimit && options.ErrorOutput != nil {
		fmt.Fprintf(options.ErrorOutput, "note: outlining the first %d files (default --limit); pass --limit N or --limit 0 for all\n", options.DefaultLimit)
	}
	maxBytes := 0
	if !options.JSON {
		maxBytes = options.MaxOutputBytes
	}
	output := cliruntime.NewOutput(options.Output)
	if maxBytes > 0 {
		output = cliruntime.NewBoundedOutput(options.Output, maxBytes)
	}
	outlines, printed, err := outlineFiles(options.Depth, options.Kinds, paths, options.JSON, output)
	if err != nil && !errors.Is(err, cliruntime.ErrOutputTruncated) {
		return false, err
	}
	if options.JSON {
		return true, output.WriteJSON(map[string]any{"files": outlines})
	}
	return printed, nil
}

func outlineFiles(depth int, kinds OutlineKinds, paths []string, jsonMode bool, output *cliruntime.Output) ([]parser.FileOutline, bool, error) {
	outlines := make([]parser.FileOutline, 0, len(paths))
	printed := false
	for _, path := range paths {
		outline, data, ok := loadOutline(path, depth)
		if !ok {
			continue
		}
		outline = FilterOutline(outline, kinds)
		if jsonMode {
			outlines = append(outlines, outline)
			continue
		}
		if len(outline.Symbols) == 0 {
			continue
		}
		if printed {
			if err := output.WriteString("\n"); err != nil {
				return nil, false, err
			}
		}
		if err := output.WriteString(outlineOutput(outline, string(data), len(kinds) > 0)); err != nil {
			return nil, false, err
		}
		printed = true
	}
	return outlines, printed, nil
}

func loadOutline(path string, depth int) (parser.FileOutline, []byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return parser.FileOutline{}, nil, false
	}
	return parser.OutlineFileDepth(path, string(data), depth), data, true
}
