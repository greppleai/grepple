package render

import (
	"fmt"
	"github.com/greppleai/grepple/internal/search"
)

type countRenderer struct {
	output *outputWriter
	json   bool
}

func (renderer countRenderer) Render(results []search.FileResult) error {
	type fileCount struct {
		Path  string `json:"path"`
		Count int    `json:"count"`
	}
	counts := make([]fileCount, 0, len(results))
	for _, result := range results {
		counts = append(counts, fileCount{Path: result.Path, Count: len(result.Matches)})
	}
	if renderer.json {
		return renderer.output.writeJSON(map[string]any{"counts": counts})
	}
	for _, count := range counts {
		if err := renderer.output.writeString(fmt.Sprintf("%s\t%d\n", count.Path, count.Count)); err != nil {
			return err
		}
	}
	return nil
}
