package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUserSettingsFileRemainsPrivateAndStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	settings, err := LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveUserSettings(UserSettings{Anchors: Anchors{DefaultProvider: "native"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions: %v, %v", info, err)
	}
	loaded, err := LoadConfig("", true)
	if err != nil || loaded.User.Anchors.DefaultProvider != "native" {
		t.Fatalf("loaded settings: %+v, %v", loaded, err)
	}
	if err := os.WriteFile(path, []byte(`{"anchors":{},"token":"not-a-user-setting"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig("", true); err == nil {
		t.Fatal("user settings accepted a credential field")
	}
}

func TestAskPreferencesKeepLegacyUserFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".grepple", "grepple.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"ask":{"model":"example-model","logs":{"enabled":false,"retentionPeriod":"2d"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadConfig("", true)
	if err != nil || settings.Ask.Model != "example-model" || settings.Ask.LogsEnabled || settings.Ask.LogRetention.Hours() != 48 {
		t.Fatalf("legacy Ask settings: %+v, %v", settings, err)
	}
}

func TestAskRetentionValidation(t *testing.T) {
	for value, want := range map[string]time.Duration{"7d": 7 * 24 * time.Hour, "168h": 7 * 24 * time.Hour, "30m": 30 * time.Minute} {
		got, err := parseLogRetention(value)
		if err != nil || got != want {
			t.Fatalf("retention %q=%s err=%v, want %s", value, got, err, want)
		}
	}
	for _, value := range []string{"", "0d", "-1h", "forever"} {
		if _, err := parseLogRetention(value); err == nil {
			t.Fatalf("retention %q succeeded", value)
		}
	}
}
