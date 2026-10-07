package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/aiprovider"
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
	if err := mustBackendConfig(t).StoreBackendLogin("token", "", 0, 0, "user", ""); err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	if err := NewLogout(cliruntime.Environment{ErrorOutput: &diagnostics}).Run(nil); err != nil {
		t.Fatal(err)
	}
	if token := mustBackendConfig(t).AuthToken(); token != "" || !strings.Contains(diagnostics.String(), "Logged out") {
		t.Fatalf("token=%q diagnostics=%q", token, diagnostics.String())
	}
}

func TestLogoutSurvivesMalformedUserPreferences(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := mustBackendConfig(t).StoreBackendLogin("token", "", 0, 0, "user", ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	if err := os.WriteFile(path, []byte(`{"anchors":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewLogout(cliruntime.Environment{}).Run(nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_SETTINGS", "")
	if token := mustBackendConfig(t).AuthToken(); token != "" {
		t.Fatalf("logout left token %q", token)
	}
}
