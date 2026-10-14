package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAskSettingsDefaultsOverridesAndValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".grepple"), 0700); err != nil {
		t.Fatal(err)
	}
	// The retired file is not loaded, even when it is malformed.
	if err := os.WriteFile(filepath.Join(home, ".grepple", "grepple.json"), []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	for _, test := range []struct {
		name, content string
		invalid       bool
	}{
		{"defaults", `{}`, false},
		{"empty ask", `{"ask":{}}`, false},
		{"model", `{"ask":{"model":" example "}}`, false},
		{"invalid duration", `{"ask":{"logs":{"retentionPeriod":"forever"}}}`, true},
		{"unknown ask field", `{"ask":{"unknown":true}}`, true},
		{"unknown logs field", `{"ask":{"logs":{"unknown":true}}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := LoadConfig("", true)
			if test.invalid {
				if err == nil || !strings.Contains(err.Error(), path) {
					t.Fatalf("expected settings error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !settings.Ask.LogsEnabled || settings.Ask.LogRetention != 7*24*time.Hour {
				t.Fatalf("defaults: %+v", settings.Ask)
			}
			want := ""
			if test.name == "model" {
				want = "example"
			}
			if settings.Ask.Model != want {
				t.Fatalf("model %q, want %q", settings.Ask.Model, want)
			}
		})
	}
}
