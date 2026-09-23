package graph

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestResolveCommandBuildsItsGraph(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.go")
	if err := os.WriteFile(path, []byte("package service\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	exitCode := 0
	command := New(cliruntime.Environment{Output: &output, Exit: func(code int) { exitCode = code }})
	if err := command.Run([]string{"resolve", "--json", "--symbol", "Run", "--max-files", "3", path}); err != nil {
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
	path := filepath.Join(t.TempDir(), "service.go")
	if err := os.WriteFile(path, []byte("package service\nfunc Present() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	exitCode := 0
	command := New(cliruntime.Environment{Output: &output, Exit: func(code int) { exitCode = code }})
	if err := command.Run([]string{"resolve", "--json", "--symbol", "Missing", path}); err != nil {
		t.Fatal(err)
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	output.Reset()
	exitCode = 0
	if err := command.Run([]string{"resolve", "--symbol", "Missing", path}); err != nil {
		t.Fatal(err)
	}
	if exitCode != 1 || output.String() != "no matches for Missing\n" {
		t.Fatalf("human no-match output=%q exit=%d", output.String(), exitCode)
	}
}
