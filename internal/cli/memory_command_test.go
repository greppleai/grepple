package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	memorycommand "github.com/greppleai/grepple/internal/cli/memory"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestMemoryCLICommands(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("a.go", []byte("package a\nfunc Run() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("internal", "cli"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("body.txt", []byte("full private note\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"memory"}); err == nil || !strings.Contains(err.Error(), "requires store") {
		t.Fatalf("no-subcommand error: %v", err)
	}
	if err := Run([]string{"memory", "store", "sample", "--description", "A short hint", "--body-file", "body.txt", "--symbol", "a.go@func:2-2"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"memory", "store", "sample", "--description", "Hint", "--body-file", "body.txt", "--keyword", "source"}); err == nil {
		t.Fatal("store overwrote")
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"memory", "recall", "sample"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "full private note") || !strings.Contains(output, "@func:2-2") {
		t.Fatalf("recall %q", output)
	}
	if err := Run([]string{"memory", "update", "sample", "--description", "Replacement", "--body-file", "body.txt", "--keyword", "source"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"memory", "store", "scoped", "--description", "Scope hint", "--body-file", "body.txt", "--scope", "internal/cli", "--scope", "internal/cli/*.go", "--scope", "internal/cli/**/*.go"}); err != nil {
		t.Fatal(err)
	}
	scoped := captureStdout(t, func() {
		if err := Run([]string{"memory", "recall", "scoped"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, scope := range []string{"internal/cli/", "internal/cli/*.go", "internal/cli/**/*.go"} {
		if !strings.Contains(scoped, scope) {
			t.Fatalf("missing stored scope %q in %q", scope, scoped)
		}
	}
	if err := Run([]string{"memory", "update", "scoped", "--description", "Replacement", "--body-file", "body.txt", "--keyword", "source"}); err != nil {
		t.Fatal(err)
	}
	scoped = captureStdout(t, func() {
		if err := Run([]string{"memory", "recall", "scoped"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(scoped, "scopes:") {
		t.Fatalf("update retained scopes: %q", scoped)
	}
	if err := Run([]string{"memory", "delete", "sample"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".grepple", "memories", "sample.md")); !os.IsNotExist(err) {
		t.Fatalf("entry remains: %v", err)
	}
	for _, args := range [][]string{{"memory", "store", "bad/name", "--description", "Hint", "--body-file", "body.txt", "--keyword", "source"}, {"memory", "store", "other", "--description", "Hint", "--body-file", "body.txt", "--file", "../escape.go"}, {"memory", "store", "other", "--description", "Hint", "--body-file", "body.txt", "--scope", "a.go"}, {"memory", "store", "other", "--description", "Hint", "--body-file", "body.txt", "--remote"}} {
		if err := Run(args); err == nil {
			t.Fatalf("accepted invalid args %q", args)
		}
	}
}

func TestMemoryStdinAndHelp(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(".git", 0700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx := cliruntime.Environment{Input: strings.NewReader("stdin private body"), Output: &output}
	err := memorycommand.Execute(ctx, &memorycommand.Args{Store: &memorycommand.WriteArgs{Name: "piped", Description: "Short", BodyFile: "-", Keywords: []string{"source"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".grepple", "memories", "piped.md")); err != nil {
		t.Fatal(err)
	}
	help := captureStdout(t, func() {
		if err := Run([]string{"help", "memory"}); err != nil {
			t.Fatal(err)
		}
	})
	storeHelp := captureStdout(t, func() {
		if err := Run([]string{"help", "memory", "store"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(help, "recall") || !strings.Contains(storeHelp, "--body-file") || !strings.Contains(storeHelp, "--scope") {
		t.Fatalf("memory help: %q", help)
	}
}
