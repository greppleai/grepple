package extract

import (
	"strings"
	"testing"
)

func TestFlowParserAllowsOrdinaryComments(t *testing.T) {
	flow := "%% architecture overview\nflowchart LR\n %% owner: platform\n start[Start]\n %% this is ordinary prose\n"
	if _, err := ParseFlowchart(flow); err != nil {
		t.Fatalf("ordinary comments must be allowed: %v", err)
	}
}

func TestFlowParserRejectsUnknownAndMalformedMetadataAnywhere(t *testing.T) {
	tests := []struct {
		name, flow, want string
	}{
		{"unknown before header", "%% grepple:endpoint start\nflowchart LR\n start[Start]\n", "unknown metadata directive"},
		{"unknown after header", "flowchart LR\n start[Start]\n %% pi:endpoint start\n", "unknown metadata directive"},
		{"malformed before header", "%% pi:symbol start\nflowchart LR\n start[Start]\n", "malformed symbol directive"},
		{"malformed after header", "flowchart LR\n start[Start]\n %% grepple:language start swift\n", "malformed language directive"},
		{"empty directive", "flowchart LR\n %% grepple:\n", "malformed metadata directive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseFlowchart(test.flow)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestFlowParserRejectsDuplicateAndConflictingNodeMetadata(t *testing.T) {
	tests := []struct {
		name, directives, want string
	}{
		{"symbol", "%% grepple:symbol node one\n%% pi:symbol node two", "duplicate symbol"},
		{"package", "%% grepple:package node example.com/one\n%% pi:package node example.com/two", "duplicate package"},
		{"module", "%% grepple:module node one.ts\n%% pi:module node two.ts", "duplicate module"},
		{"language", "%% grepple:language node go\n%% pi:language node go", "duplicate language"},
		{"concept", "%% grepple:concept node\n%% pi:concept node", "duplicate concept"},
		{"package module", "%% grepple:package node example.com/app\n%% pi:module node app.ts", "both package and module"},
		{"language package", "%% grepple:language node typescript\n%% pi:package node example.com/app", "conflicts"},
		{"module language", "%% grepple:module node app.ts\n%% pi:language node go", "conflicts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flow := "flowchart LR\nnode[Node]\n" + test.directives + "\n"
			_, err := ParseFlowchart(flow)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}
