package auth

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestAIProviderListShowsBuiltInsLoggedOut(t *testing.T) {
	t.Setenv(aiprovider.CredentialsPathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	for _, name := range []string{"GREPPLE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", "GREPPLE_ANTHROPIC_OAUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "GREPPLE_OPENAI_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(name, "")
	}
	var output bytes.Buffer
	if err := NewAIProvider(cliruntime.Environment{Output: &output}).Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	want := "anthropic\tlogged-out\nanthropic-subscription\tlogged-out\nbedrock\tlogged-out\ncodex\tlogged-out\ncopilot\tlogged-out\nopenai\tlogged-out\n"
	if output.String() != want {
		t.Fatalf("output=%q", output.String())
	}
}
