package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

type compactQueryEdge struct {
	call     parser.NavigationCall
	other    parser.NavigationDeclaration
	arrow    string
	targetID string
}

// writeCompactDirectionalQuery groups each call under its selected root, walking
// incoming, outgoing, or both directions. Confidence belongs to the edge, not
// the declaration; candidate targets remain marked as ambiguous.
func writeCompactDirectionalQuery(write func(string) bool, query Query, declarationList []parser.NavigationDeclaration, calls []parser.NavigationCall) {
	declarations := make(map[string]parser.NavigationDeclaration, len(declarationList))
	for _, declaration := range declarationList {
		declarations[declaration.ID] = declaration
	}
	adjacent := make(map[string][]compactQueryEdge)
	for _, call := range calls {
		targetIDs := call.CandidateTargetIDs
		if call.TargetID != "" {
			targetIDs = []string{call.TargetID}
		}
		caller, callerFound := declarations[call.CallerID]
		for _, targetID := range targetIDs {
			target, targetFound := declarations[targetID]
			if !callerFound || !targetFound {
				continue
			}
			if query.Direction == "callers" || query.Direction == "impact" {
				adjacent[targetID] = append(adjacent[targetID], compactQueryEdge{call: call, other: caller, arrow: "<-", targetID: targetID})
			}
			if query.Direction != "callers" {
				adjacent[call.CallerID] = append(adjacent[call.CallerID], compactQueryEdge{call: call, other: target, arrow: "->", targetID: targetID})
			}
		}
	}
	for id := range adjacent {
		sort.SliceStable(adjacent[id], func(i, j int) bool {
			left, right := adjacent[id][i], adjacent[id][j]
			if left.arrow != right.arrow {
				return left.arrow == "<-"
			}
			if left.other.Path != right.other.Path {
				return left.other.Path < right.other.Path
			}
			if left.other.Start != right.other.Start {
				return left.other.Start < right.other.Start
			}
			if left.call.Path != right.call.Path {
				return left.call.Path < right.call.Path
			}
			if left.call.Line != right.call.Line {
				return left.call.Line < right.call.Line
			}
			return left.call.ID < right.call.ID
		})
	}
	for _, rootID := range query.RootIDs {
		root, ok := declarations[rootID]
		if !ok || !write(fmt.Sprintf("%s %s %s %s", root.Language, root.Kind, root.Name, graphDeclarationLocation(root))) {
			return
		}
		seen := map[string]bool{rootID: true}
		shownEdges := make(map[string]bool)
		var walk func(string, int) bool
		walk = func(id string, depth int) bool {
			if depth >= query.Depth {
				return true
			}
			next := make([]string, 0)
			for _, edge := range adjacent[id] {
				edgeID := edge.call.ID + ":" + edge.targetID
				if query.Direction == "impact" && shownEdges[edgeID] {
					continue
				}
				label := ""
				if edge.call.TargetID == "" {
					label = " ?"
				}
				callLocation := fmt.Sprint(edge.call.Line)
				if caller, ok := declarations[edge.call.CallerID]; !ok || caller.Path != edge.call.Path {
					callLocation = fmt.Sprintf("%s:%d", edge.call.Path, edge.call.Line)
				}
				line := fmt.Sprintf("%s%s%s %s %s %s call:%s [%s]", strings.Repeat("  ", depth), edge.arrow, label, edge.other.Kind, edge.other.Name, graphDeclarationLocation(edge.other), callLocation, edge.call.Confidence)
				if seen[edge.other.ID] {
					line += " (already shown)"
				}
				if !write(line) {
					return false
				}
				shownEdges[edgeID] = true
				if !seen[edge.other.ID] {
					seen[edge.other.ID] = true
					if query.Direction == "impact" {
						next = append(next, edge.other.ID)
					} else if !walk(edge.other.ID, depth+1) {
						return false
					}
				}
			}
			for _, id := range next {
				if !walk(id, depth+1) {
					return false
				}
			}
			return true
		}
		if !walk(rootID, 0) {
			return
		}
	}
}
