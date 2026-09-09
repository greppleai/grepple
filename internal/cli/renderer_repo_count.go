package cli

import (
	"fmt"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

type repoCountRenderer struct {
	output *outputWriter
	json   bool
}

func (renderer repoCountRenderer) Render(counts []api.RepoCount) error {
	search.SortRepoCounts(counts)
	totalFiles, totalMatches := 0, 0
	for _, count := range counts {
		totalFiles += count.Files
		totalMatches += count.Matches
	}
	if renderer.json {
		return renderer.renderJSON(counts, totalFiles, totalMatches)
	}
	return renderer.renderText(counts, totalFiles, totalMatches)
}

func (renderer repoCountRenderer) renderJSON(counts []api.RepoCount, totalFiles, totalMatches int) error {
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

func (renderer repoCountRenderer) renderText(counts []api.RepoCount, totalFiles, totalMatches int) error {
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
