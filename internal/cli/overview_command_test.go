package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestOverviewDispatchHelpAndLocalBoundary(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"overview"}, {"overview", "--json"}} {
		values, help, err := parseApplicationArgs(args)
		if err != nil || help || values.Overview == nil || values.Overview.JSON != (len(args) == 2) || values.traceCommandName() != "overview" {
			t.Fatalf("parse %q: %+v help=%v err=%v", args, values, help, err)
		}
		var output bytes.Buffer
		application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}})
		if err := executeArguments(application, values); err != nil {
			t.Fatal(err)
		}
		if len(args) == 2 && !strings.Contains(output.String(), `"schema": "grepple-overview-v1"`) {
			t.Fatalf("JSON route: %s", output.String())
		}
		if len(args) == 1 && !strings.Contains(output.String(), "grepple verify") {
			t.Fatalf("text route: %s", output.String())
		}
	}
	helpOutput := captureStdout(t, func() {
		if err := Run([]string{"help", "overview"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(helpOutput, "Usage: grepple overview") || !strings.Contains(helpOutput, "--json") {
		t.Fatalf("overview help: %s", helpOutput)
	}
	if err := validateCommandAvailability("overview", []string{"--repo", "owner/repo"}); err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Fatalf("remote overview allowed: %v", err)
	}
}
