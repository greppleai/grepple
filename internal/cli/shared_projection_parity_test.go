package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestSharedNavigationEdgeParityAcrossAgentProjections(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "go.mod", "module example.com/parity\n")
	writeGraphSource(t, root, "service.go", "package parity\ntype Worker struct{}\nfunc (Worker) Run() { Helper() }\nfunc Helper() {}\n")
	graph := loadParityJSONGraph(t)
	caller, target, call := parityGraphEdge(t, graph, "Worker.Run", "Helper")
	assertParityCompactGraph(t, caller, target)
	assertParityFocusedFlow(t)
	assertParityRelatedOutput(t, call)
	assertParityArchitectureResolve(t)
}

func loadParityJSONGraph(t *testing.T) navigationGraphOutput {
	t.Helper()
	jsonText := captureStdout(t, func() {
		if err := Run([]string{"graph", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var graph navigationGraphOutput
	if err := json.Unmarshal([]byte(jsonText), &graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

func assertParityCompactGraph(t *testing.T, caller, target parser.NavigationDeclaration) {
	t.Helper()
	compact := captureStdout(t, func() {
		if err := Run([]string{"graph", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	assertContainsAll(t, compact, []string{shortGraphID(caller.ID), shortGraphID(target.ID), "Worker.Run -> Helper#" + shortGraphID(target.ID)}, "compact graph")
}

func assertParityFocusedFlow(t *testing.T) {
	t.Helper()
	flow := captureStdout(t, func() {
		if err := Run([]string{"extract", "flow", "--at", "service.go:3", "--source", ".", "--depth", "1"}); err != nil {
			t.Fatal(err)
		}
	})
	assertContainsAll(t, flow, []string{"Worker_Run --> Helper", "%% grepple:symbol Worker_Run Worker.Run", "service.go:3"}, "focused Mermaid")
}

func assertParityRelatedOutput(t *testing.T, call parser.NavigationCall) {
	t.Helper()
	related := captureStdout(t, func() {
		if err := Run([]string{"--related", "--at", "service.go:3"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(related, "→ Helper") || !strings.Contains(related, "service.go:4") {
		t.Fatalf("related output disagrees with graph call %#v:\n%s", call, related)
	}
}

func assertParityArchitectureResolve(t *testing.T) {
	t.Helper()
	resolved := captureStdout(t, func() {
		if err := Run([]string{"architecture", "resolve", "--symbol", "Worker", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	assertContainsAll(t, resolved, []string{"matches=1", "go struct Worker service.go:2"}, "architecture resolve")
}

func assertContainsAll(t *testing.T, content string, expected []string, projection string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("%s missing graph fact %q:\n%s", projection, value, content)
		}
	}
}

func parityGraphEdge(t *testing.T, graph navigationGraphOutput, callerName, targetName string) (parser.NavigationDeclaration, parser.NavigationDeclaration, parser.NavigationCall) {
	t.Helper()
	byName := make(map[string]parser.NavigationDeclaration, len(graph.Declarations))
	for _, declaration := range graph.Declarations {
		byName[declaration.Name] = declaration
	}
	caller, target := byName[callerName], byName[targetName]
	for _, call := range graph.Calls {
		if call.CallerID == caller.ID && call.TargetID == target.ID {
			return caller, target, call
		}
	}
	t.Fatalf("missing resolved %s -> %s edge: declarations=%#v calls=%#v", callerName, targetName, graph.Declarations, graph.Calls)
	return parser.NavigationDeclaration{}, parser.NavigationDeclaration{}, parser.NavigationCall{}
}
