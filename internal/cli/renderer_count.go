package cli

import (
	"fmt"

	"github.com/greppleai/grepple/api"
)

type countRenderer struct {
	output *outputWriter
	json   bool
}

func (renderer countRenderer) Render(results []api.FileResult) error {
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
