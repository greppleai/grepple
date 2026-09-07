package search

import (
	"grepple/internal/api"
	"sort"
)

// ContextLines selects the matching lines (capped at limit, in line order)
// plus n lines of context around each, merging overlaps; each returned line is
// flagged as a match or context.
func ContextLines(content string, hits map[int]bool, n, limit int) []api.ContextLine {
	lines := SplitLines(content)
	var hs []int
	for x := range hits {
		hs = append(hs, x)
	}
	sort.Ints(hs)
	if len(hs) > limit {
		hs = hs[:limit]
	}
	chosen := map[int]bool{}
	inc := map[int]bool{}
	for _, x := range hs {
		chosen[x] = true
		for i := max(1, x-n); i <= min(len(lines), x+n); i++ {
			inc[i] = true
		}
	}
	var ns []int
	for x := range inc {
		ns = append(ns, x)
	}
	sort.Ints(ns)
	out := make([]api.ContextLine, 0, len(ns))
	for _, x := range ns {
		out = append(out, api.ContextLine{Line: x, Text: lines[x-1], Match: chosen[x]})
	}
	return out
}
