package ask

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/usersettings"
)

func TestConfiguredAskPreferencesUsesAskScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, ".grepple")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "grepple.json")
	content := `{"ask":{"model":" anthropic/claude-sonnet ","logs":{"enabled":false,"retentionPeriod":"3d"}},"ai":{"model":"codex/ignored"},"future":true}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	preferences, err := usersettings.LoadAskPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if preferences.Model != "anthropic/claude-sonnet" || preferences.LogsEnabled || preferences.LogRetention != 72*time.Hour {
		t.Fatalf("preferences=%+v", preferences)
	}
	provider, selected, err := aiprovider.ResolveSelection("", "", preferences.Model)
	if err != nil || provider != "anthropic" || selected != "claude-sonnet" {
		t.Fatalf("provider=%q model=%q err=%v", provider, selected, err)
	}
	if err := os.WriteFile(path, []byte(`{"ask":{"model":"codex/luna"}} trailing`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := usersettings.LoadAskPreferences(); err == nil {
		t.Fatal("expected malformed user configuration error")
	}
}

func TestConfiguredAskPreferencesDefaultsAndValidatesRetention(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	preferences, err := usersettings.LoadAskPreferences()
	if err != nil || !preferences.LogsEnabled || preferences.LogRetention != 7*24*time.Hour {
		t.Fatalf("defaults=%+v err=%v", preferences, err)
	}
	for value, want := range map[string]time.Duration{"7d": 7 * 24 * time.Hour, "168h": 7 * 24 * time.Hour, "30m": 30 * time.Minute} {
		got, err := usersettings.ParseLogRetention(value)
		if err != nil || got != want {
			t.Fatalf("retention %q=%s err=%v, want %s", value, got, err, want)
		}
	}
	for _, value := range []string{"", "0d", "-1h", "forever"} {
		if _, err := usersettings.ParseLogRetention(value); err == nil {
			t.Fatalf("retention %q succeeded", value)
		}
	}
}

func TestResolveAskSelectionPrecedenceAndCompatibility(t *testing.T) {
	tests := []struct {
		name, provider, explicit, configured string
		wantProvider, wantModel              string
		wantError                            bool
	}{
		{name: "provider prefixed explicit", explicit: "copilot/gpt-4.1", configured: "anthropic/claude", wantProvider: "copilot", wantModel: "gpt-4.1"},
		{name: "matching explicit provider", provider: "anthropic", explicit: "anthropic/claude", wantProvider: "anthropic", wantModel: "claude"},
		{name: "conflicting explicit provider", provider: "openai", explicit: "anthropic/claude", wantError: true},
		{name: "configured selector", configured: "openai/gpt-5.1", wantProvider: "openai", wantModel: "gpt-5.1"},
		{name: "explicit provider ignores other config", provider: "copilot", configured: "openai/gpt-5.1", wantProvider: "copilot"},
		{name: "legacy config remains codex", configured: "gpt-5.6-luna", wantProvider: "codex", wantModel: "gpt-5.6-luna"},
		{name: "unprefixed explicit defaults codex", explicit: "gpt-5.1", wantProvider: "codex", wantModel: "gpt-5.1"},
		{name: "malformed selector", configured: "copilot/", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, model, err := aiprovider.ResolveSelection(test.provider, test.explicit, test.configured)
			if (err != nil) != test.wantError || provider != test.wantProvider || model != test.wantModel {
				t.Fatalf("provider=%q model=%q err=%v", provider, model, err)
			}
		})
	}
}
