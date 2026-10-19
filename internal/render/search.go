package render

import (
	"fmt"
	"io"

	"github.com/greppleai/grepple/internal/search"
)

// SearchOptions supplies result-formatting and warning destinations.
type SearchOptions struct {
	Options
	Output      io.Writer
	ErrorOutput io.Writer
}

// Search renders results and reports line-range warnings.
func Search(options SearchOptions, results []search.FileResult) error {
	warnLineRangeResults(results, options.ErrorOutput)
	_, omitted, err := renderWithMemory(options.Options, results, options.Output)
	if omitted && options.ErrorOutput != nil {
		fmt.Fprintln(options.ErrorOutput, "warning: memory references omitted by --max-output-bytes")
	}
	return err
}

func warnLineRangeResults(results []search.FileResult, destination io.Writer) {
	if destination == nil {
		return
	}
	seen := map[string]bool{}
	for _, result := range results {
		if result.LineRange == nil || result.LineRange.Warning == "" {
			continue
		}
		warning := result.Path + ": " + result.LineRange.Warning
		if !seen[warning] {
			fmt.Fprintln(destination, "warning:", warning)
			seen[warning] = true
		}
	}
}
