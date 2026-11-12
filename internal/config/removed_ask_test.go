package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserSettingsWithoutAsk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	for _, content := range []string{`{}`, `{"anchors":{"default_provider":"native"}}`} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig("", true); err != nil {
			t.Fatalf("settings without ask: %v", err)
		}
	}
}

func TestUserSettingsRejectRemovedAsk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	if err := os.WriteFile(path, []byte(`{"ask":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig("", true); err == nil || !strings.Contains(err.Error(), `unknown field "ask"`) {
		t.Fatalf("removed setting error = %v", err)
	}
}
