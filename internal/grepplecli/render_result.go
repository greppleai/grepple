package grepplecli

import "grepple/internal/api"

// FlatMatches flattens file results into one object per matching line
// (path, line, text, plus repo when set) for --json-matches output.
func FlatMatches(results []api.FileResult) []map[string]any {
	out := []map[string]any{}
	for _, result := range results {
		for _, match := range result.Matches {
			item := map[string]any{"path": result.Path, "line": match.Line, "text": match.Text}
			if result.Repo != "" {
				item["repo"] = result.Repo
			}
			out = append(out, item)
		}
	}
	return out
}
