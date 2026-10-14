package navigation

import "github.com/greppleai/grepple/internal/parser"

func assignSnapshotCallDetails(graph *parser.NavigationGraph, records []parser.NavigationCall) {
	graph.Calls = append([]parser.NavigationCall(nil), records...)
	for i := range graph.Calls {
		if graph.Calls[i].TargetID == "" {
			graph.Calls[i].CandidateTargetIDs = nil
		}
		graph.Calls[i].CallerID = ""
		graph.Calls[i].TargetID = ""
	}
}
