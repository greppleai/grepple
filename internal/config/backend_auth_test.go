package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBackendLoginPreservesServerAndUsesPrivateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".grepple", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"server":"https://user.example"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := settings.BackendAuthPath(); err != nil || got != path {
		t.Fatalf("backend auth path = %q, %v", got, err)
	}
	if err := settings.StoreBackendLogin("token", "refresh", 3600, 7200, "user"); err != nil {
		t.Fatal(err)
	}
	if got := settings.BackendCredentials(); got.Token != "token" || got.RefreshToken != "refresh" || got.TokenExpiry == 0 || got.RefreshExpiry == 0 || got.User != "user" {
		t.Fatalf("saved backend credentials: %+v", got)
	}
	assertBackendFile(t, path, "https://user.example", "token", "refresh")
	if err := settings.ClearBackendLogin(); err != nil {
		t.Fatal(err)
	}
	if settings.AuthToken() != "" || settings.BackendCredentials().User != "" {
		t.Fatal("logout left backend credentials in the snapshot")
	}
	assertBackendFile(t, path, "https://user.example", "", "")
}

func assertBackendFile(t *testing.T, path, server, token, refresh string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved backendAuthState
	if err := json.Unmarshal(content, &saved); err != nil || saved.Server != server || saved.Token != token || saved.RefreshToken != refresh {
		t.Fatalf("backend auth contents: %+v, %v", saved, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backend auth permissions: %v, %v", info, err)
	}
}

func TestMalformedUserSettingsDoNotBlockBackendLogout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings, err := LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.StoreBackendLogin("token", "", 0, 0, "user"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	if err := os.WriteFile(path, []byte(`{"anchors":`), 0o600); err != nil {
		t.Fatal(err)
	}
	incomplete, err := LoadConfig("", true)
	if err == nil || incomplete == nil || incomplete.AuthToken() != "token" {
		t.Fatalf("backend auth missing from incomplete config: %+v, %v", incomplete, err)
	}
	if err := incomplete.ClearBackendLogin(); err != nil {
		t.Fatal(err)
	}
	incomplete, err = LoadConfig("", true)
	if err == nil || incomplete == nil || incomplete.AuthToken() != "" {
		t.Fatalf("logout failed with malformed settings: %+v, %v", incomplete, err)
	}
}

func TestBackendLogoutDoesNotIgnoreUnreadableState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	settings, err := LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	path, err := settings.BackendAuthPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := settings.ClearBackendLogin(); err == nil {
		t.Fatal("logout silently ignored an unreadable backend auth path")
	}
}
