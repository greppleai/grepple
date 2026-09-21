package graph

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestResolveCommandUsesInjectedGraph(t *testing.T) {
	var output bytes.Buffer
	exitCode := 0
	command := New(Dependencies{Stdout: &output, LoadOutput: func(paths []string, maxFiles int) (Output, error) {
		if len(paths) != 1 || paths[0] != "scope" || maxFiles != 3 {
			t.Fatalf("LoadOutput(%v, %d)", paths, maxFiles)
		}
		return Output{Schema: navigationGraphSchema, Sources: SourceSummary{Discovered: 1, Selected: 1, Parsed: 1}, Declarations: []parser.NavigationDeclaration{{ID: "go:function:scope/service.go:1:Run", Name: "Run", Kind: "function", Language: "go", Path: "scope/service.go", Start: 1, End: 2, Visibility: parser.NavigationVisibilityPublic}}}, nil
	}, RequestExit: func(code int) { exitCode = code }})
	if err := command.Run([]string{"resolve", "--json", "--symbol", "Run", "--max-files", "3", "scope"}); err != nil {
		t.Fatal(err)
	}
	var response ResolveOutput
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Matches) != 1 || response.Matches[0].Name != "Run" || exitCode != 0 {
		t.Fatalf("response=%#v exit=%d", response, exitCode)
	}
}

func TestResolveCommandRequestsNonzeroExitForNoMatches(t *testing.T) {
	var output bytes.Buffer
	exitCode := 0
	command := New(Dependencies{Stdout: &output, LoadOutput: func([]string, int) (Output, error) {
		return Output{Declarations: []parser.NavigationDeclaration{}}, nil
	}, RequestExit: func(code int) { exitCode = code }})
	if err := command.Run([]string{"resolve", "--json", "--symbol", "Missing"}); err != nil {
		t.Fatal(err)
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
}
