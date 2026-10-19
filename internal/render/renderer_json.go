package render

import (
	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/resultanalysis"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

type jsonResultRenderer struct {
	output         *outputWriter
	matchesOnly    bool
	metadata       *wire.ResultMetadata
	annotateScopes bool
	memories       *memory.Discovery
}

func (renderer jsonResultRenderer) Render(results []search.FileResult) error {
	results = scopeDisplayResults(results, renderer.annotateScopes)
	if renderer.matchesOnly {
		response := map[string]any{"matches": flatMatches(results), "metadata": renderer.metadata}
		if ranges := resultLineRanges(results); len(ranges) > 0 {
			response["lineRanges"] = ranges
		}
		addMemoryJSON(response, renderer.memories)
		return renderer.output.writeJSON(response)
	}
	response := wire.SearchResponse{Results: results, Metadata: renderer.metadata, SourceAnalysis: searchSourceAnalysis(results)}
	if renderer.memories != nil {
		response.MemoryStatus = &wire.MemoryStatus{Complete: renderer.memories.Complete, Omitted: renderer.memories.Omitted}
		for _, ref := range renderer.memories.References {
			response.Memories = append(response.Memories, wire.MemoryReference{Name: ref.Name, Description: safeMemoryDescription(ref.Description, 120)})
		}
	}
	return renderer.output.writeJSON(response)
}
func addMemoryJSON(response map[string]any, found *memory.Discovery) {
	if found == nil {
		return
	}
	response["memoryStatus"] = wire.MemoryStatus{Complete: found.Complete, Omitted: found.Omitted}
	if len(found.References) == 0 {
		return
	}
	refs := make([]wire.MemoryReference, 0, len(found.References))
	for _, ref := range found.References {
		refs = append(refs, wire.MemoryReference{Name: ref.Name, Description: safeMemoryDescription(ref.Description, 120)})
	}
	response["memories"] = refs
}

func searchSourceAnalysis(results []search.FileResult) *wire.SourceAnalysis {
	return resultanalysis.Sources(results)
}

func flatMatches(results []search.FileResult) []map[string]any {
	matches := []map[string]any{}
	for _, result := range results {
		for _, match := range result.Matches {
			item := map[string]any{"path": result.Path, "line": match.Line, "text": match.Text}
			if match.Nearest != nil {
				item["nearest"] = match.Nearest
			}
			if match.Scope != nil {
				item["scope"] = match.Scope
			}
			if match.StartLine > 0 {
				item["startLine"] = match.StartLine
			}
			if match.EndLine > 0 {
				item["endLine"] = match.EndLine
			}
			if result.Repo != "" {
				item["repo"] = result.Repo
			}
			matches = append(matches, item)
		}
	}
	return matches
}

func resultLineRanges(results []search.FileResult) []map[string]any {
	ranges := []map[string]any{}
	for _, result := range results {
		if result.LineRange != nil {
			ranges = append(ranges, map[string]any{"path": result.Path, "range": result.LineRange})
		}
	}
	return ranges
}
