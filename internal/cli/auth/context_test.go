package auth

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/authstate"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestAIProviderCommandUsesCommandContextOutput(t *testing.T) {
	t.Setenv(aiprovider.CredentialsPathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	var output bytes.Buffer
	if err := NewAIProvider(cliruntime.Environment{Output: &output}).Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "codex\t") {
		t.Fatalf("output=%q", output.String())
	}
}

func TestLogoutCommandUsesApplicationDiagnostics(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := authstate.SaveToken("token", "user"); err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	if err := NewLogout(cliruntime.Environment{ErrorOutput: &diagnostics}).Run(nil); err != nil {
		t.Fatal(err)
	}
	if authstate.Token() != "" || !strings.Contains(diagnostics.String(), "Logged out") {
		t.Fatalf("token=%q diagnostics=%q", authstate.Token(), diagnostics.String())
	}
}
