package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

// writeCompactBuildGraph groups declarations by file and calls by owner. Each
// file path is printed once instead of repeating declaration/call IDs and the
// caller's name and path on every edge; JSON retains the full graph facts.
func writeCompactBuildGraph(write func(string) bool, declarations []parser.NavigationDeclaration, calls []parser.NavigationCall) {
	byID := make(map[string]parser.NavigationDeclaration, len(declarations))
	byPath := make(map[string][]parser.NavigationDeclaration)
	callsByCaller := make(map[string][]parser.NavigationCall)
	for _, declaration := range declarations {
		byID[declaration.ID] = declaration
		byPath[declaration.Path] = append(byPath[declaration.Path], declaration)
	}
	for _, call := range calls {
		callsByCaller[call.CallerID] = append(callsByCaller[call.CallerID], call)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		sort.Slice(byPath[path], func(i, j int) bool {
			left, right := byPath[path][i], byPath[path][j]
			if left.Start != right.Start {
				return left.Start < right.Start
			}
			if left.End != right.End {
				return left.End < right.End
			}
			return left.ID < right.ID
		})
		if !write(path) {
			return
		}
		for _, declaration := range byPath[path] {
			line := fmt.Sprintf("  %s %s %s visibility=%s", compactBuildRange(declaration.Start, declaration.End), declaration.Kind, declaration.Name, declaration.Visibility)
			if declaration.Entrypoint != "" {
				line += " entrypoint=" + declaration.Entrypoint
			}
			if !write(line) {
				return
			}
			outgoing := callsByCaller[declaration.ID]
			sort.Slice(outgoing, func(i, j int) bool {
				if outgoing[i].Path != outgoing[j].Path {
					return outgoing[i].Path < outgoing[j].Path
				}
				if outgoing[i].Line != outgoing[j].Line {
					return outgoing[i].Line < outgoing[j].Line
				}
				return outgoing[i].ID < outgoing[j].ID
			})
			for _, call := range outgoing {
				location := fmt.Sprint(call.Line)
				if call.Path != declaration.Path {
					location = fmt.Sprintf("%s:%d", call.Path, call.Line)
				}
				if !write(fmt.Sprintf("    -> %s call:%s [%s]", compactBuildTarget(call, byID, declaration.Path), location, call.Confidence)) {
					return
				}
			}
		}
	}
}

func compactBuildRange(start, end int) string {
	if start == end {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func compactBuildTarget(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration, callerPath string) string {
	location := func(target parser.NavigationDeclaration) string {
		if target.Path == callerPath {
			return target.Name + ":" + compactBuildRange(target.Start, target.End)
		}
		return target.Name + " " + graphDeclarationLocation(target)
	}
	if target, ok := declarations[call.TargetID]; ok && call.TargetID != "" {
		return location(target)
	}
	if call.TargetID != "" {
		return "? " + compactBuildFallbackName(call) + " (target unavailable)"
	}
	const maxCandidates = 3
	candidates := make([]string, 0, maxCandidates+1)
	for _, id := range call.CandidateTargetIDs {
		if len(candidates) == maxCandidates {
			break
		}
		if target, ok := declarations[id]; ok {
			candidates = append(candidates, location(target))
		} else {
			candidates = append(candidates, "unavailable:"+shortGraphID(id))
		}
	}
	if omitted := len(call.CandidateTargetIDs) - len(candidates); omitted > 0 {
		candidates = append(candidates, fmt.Sprintf("+%d", omitted))
	}
	if len(candidates) > 0 {
		return "? " + strings.Join(candidates, ", ")
	}
	return "? " + compactBuildFallbackName(call)
}

func compactBuildFallbackName(call parser.NavigationCall) string {
	if call.Display != "" {
		return call.Display
	}
	if call.Name != "" {
		return call.Name
	}
	return "unknown"
}
