package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestSharedNavigationEdgeParityAcrossAgentProjections(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "go.mod", "module example.com/parity\n")
	writeGraphSource(t, root, "service.go", "package parity\ntype Worker struct{}\nfunc (Worker) Run() { Helper() }\nfunc Helper() {}\n")
	graph := loadParityJSONGraph(t)
	caller, target, call := parityGraphEdge(t, graph, "Worker.Run", "Helper")
	assertParityCompactGraph(t, caller, target, call)
	assertParityRelatedOutput(t, call)
}

func loadParityJSONGraph(t *testing.T) navigationGraphOutput {
	t.Helper()
	jsonText := captureStdout(t, func() {
		if err := Run([]string{"graph", "callees", "--symbol", "Worker.Run", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var graph navigationGraphOutput
	if err := json.Unmarshal([]byte(jsonText), &graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

func assertParityCompactGraph(t *testing.T, caller, target parser.NavigationDeclaration, call parser.NavigationCall) {
	t.Helper()
	compact := captureStdout(t, func() {
		if err := Run([]string{"graph", "callees", "--symbol", "Worker.Run", "."}); err != nil {
			t.Fatal(err)
		}
	})
	assertContainsAll(t, compact, []string{
		caller.Signature + " @ " + caller.Path + ":3",
		target.Signature + " @ " + target.Path + ":4",
		fmt.Sprintf("call:%d [%s]", call.Line, call.Confidence),
	}, "focused graph")
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
