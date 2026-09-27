package api

import (
	"fmt"

	"github.com/greppleai/grepple/internal/extract"
)

// GenerateSourceFlow produces a validated, source-linked Mermaid call-flow
// diagram for one unambiguous entry symbol within the supplied source root.
// It is used to maintain the backend's checked-in shard lifecycle schema.
func GenerateSourceFlow(root, entry string, depth, maxNodes int) (string, error) {
	if entry == "" || depth < 0 || maxNodes < 1 {
		return "", fmt.Errorf("flow requires an entry, non-negative depth, and a positive node limit")
	}
	sources, err := extract.LoadSources([]string{root})
	if err != nil {
		return "", err
	}
	analysis, err := extract.Analyze(sources)
	if err != nil {
		return "", err
	}
	symbol := analysis.Symbols[entry]
	if symbol == nil || len(symbol.Locations) != 1 {
		return "", fmt.Errorf("entry symbol %q was not found or is ambiguous", entry)
	}
	return extract.GenerateFlowchart(entry, symbol.Locations[0].Path, sources, extract.GenerateOptions{
		Depth: depth, DepthSet: true, MaxNodes: maxNodes,
	})
}
