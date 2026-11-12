package config

import (
	"os"
	"path/filepath"
	"testing"
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
