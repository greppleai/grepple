package search

import (
	"github.com/greppleai/grepple/api"
	"sort"
)

// ContextLines selects the matching lines (capped at limit, in line order)
// plus the requested lines before and after each, merging overlaps; each returned
// line is flagged as a match or context.
func ContextLines(content string, hits map[int]bool, before, after, limit int) []api.ContextLine {
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
		for i := max(1, x-before); i <= min(len(lines), x+after); i++ {
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
