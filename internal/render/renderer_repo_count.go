package render

import (
	"fmt"
	"io"

	"github.com/greppleai/grepple/internal/search"
)

// RepoCounts renders repository count output.
func RepoCounts(counts []search.RepoCount, destination io.Writer, jsonMode, summaryOnly bool, maxBytes int) error {
	return (repoCountRenderer{output: newOutputWriter(destination, maxBytes), json: jsonMode, summaryOnly: summaryOnly}).Render(counts)
}

type repoCountRenderer struct {
	output      *outputWriter
	json        bool
	summaryOnly bool
}

func (renderer repoCountRenderer) Render(counts []search.RepoCount) error {
	search.SortRepoCounts(counts)
	totalFiles, totalMatches := 0, 0
	for _, count := range counts {
		totalFiles += count.Files
		totalMatches += count.Matches
	}
	if renderer.summaryOnly {
		return renderer.renderSummary(totalFiles, totalMatches)
	}
	if renderer.json {
		return renderer.renderJSON(counts, totalFiles, totalMatches)
	}
	return renderer.renderText(counts, totalFiles, totalMatches)
}

func (renderer repoCountRenderer) renderSummary(totalFiles, totalMatches int) error {
	if renderer.json {
		return renderer.output.writeJSON(map[string]any{"count": map[string]int{"files": totalFiles, "matches": totalMatches}})
	}
	return renderer.output.writeString(fmt.Sprintf("%d files\t%d matches\n", totalFiles, totalMatches))
}

func (renderer repoCountRenderer) renderJSON(counts []search.RepoCount, totalFiles, totalMatches int) error {
	repos := map[string]any{}
	for _, count := range counts {
		if count.Repo != "" {
			repos[count.Repo] = map[string]any{"files": count.Files, "matches": count.Matches}
		}
	}
	return renderer.output.writeJSON(map[string]any{
		"count": map[string]any{"files": totalFiles, "matches": totalMatches, "repos": repos},
	})
}

func (renderer repoCountRenderer) renderText(counts []search.RepoCount, totalFiles, totalMatches int) error {
	printedRepo := false
	for _, count := range counts {
		if count.Repo == "" {
			continue
		}
		printedRepo = true
		if err := renderer.output.writeString(fmt.Sprintf("%s\t%d files\t%d matches\n", count.Repo, count.Files, count.Matches)); err != nil {
			return err
		}
	}
	if printedRepo {
		return renderer.output.writeString(fmt.Sprintf("total\t%d files\t%d matches\n", totalFiles, totalMatches))
	}
	return renderer.output.writeString(fmt.Sprintf("%d files\t%d matches\n", totalFiles, totalMatches))
}
