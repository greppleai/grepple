package ask

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestRunComposesResolvedCommandWithSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GREPPLE_ASK_LOG_DIR", filepath.Join(home, "logs"))
	var output, diagnostics bytes.Buffer
	called := false
	application := cliruntime.Environment{Output: &output, ErrorOutput: &diagnostics, RepositoryContext: cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return home }}}
	err := newWithSessionRunner(application, func(_ context.Context, _ *agent.Log, provider aiprovider.Provider, request SessionRequest) (string, error) {
		called = true
		if provider.Name() != "codex" || request.Model != "gpt-test" || request.Question != "find parser" || request.Timeout != 5 {
			t.Fatalf("provider=%s request=%+v", provider.Name(), request)
		}
		return "source-backed answer", nil
	}).Run([]string{"--provider", "codex", "--model", "gpt-test", "--timeout-seconds", "5", "find", "parser"})
	if err != nil {
		t.Fatal(err)
	}
	if !called || output.String() != "source-backed answer\n" || diagnostics.Len() == 0 {
		t.Fatalf("called=%v output=%q diagnostics=%q", called, output.String(), diagnostics.String())
	}
}

func TestResolveSelectionRejectsConflictingProviders(t *testing.T) {
	if _, _, err := aiprovider.ResolveSelection("codex", "anthropic/claude", ""); err == nil {
		t.Fatal("expected provider conflict")
	}
	provider, model, err := aiprovider.ResolveSelection("", "copilot/gpt-5", "")
	if err != nil || provider != "copilot" || model != "gpt-5" {
		t.Fatalf("provider=%q model=%q err=%v", provider, model, err)
	}
}

func TestCommandHelpUsesCommandContextOutput(t *testing.T) {
	var output bytes.Buffer
	if err := New(cliruntime.Environment{Output: &output}).Run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Usage: grepple ask") {
		t.Fatalf("help=%q", output.String())
	}
}
