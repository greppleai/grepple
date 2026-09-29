package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSnapshotsRepositoryAndPreservesServerPrecedence(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "nested")
	if err := os.MkdirAll(filepath.Join(root, ".grepple"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".grepple", "grepple.json")
	if err := os.WriteFile(path, []byte(`{"server":"https://repo.example","ignore":{"paths":["generated/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GREPPLE_SERVER", "")
	if err := os.MkdirAll(filepath.Join(home, ".grepple"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grepple", "config.json"), []byte(`{"server":"https://user.example","token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadConfig(child, false)
	if err != nil {
		t.Fatal(err)
	}
	if settings.RepositoryPath != path || settings.Repository.Ignore.Paths[0] != "generated/**" {
		t.Fatalf("repository config: %+v", settings)
	}
	if got := settings.ServerDefault(""); got != "https://repo.example" {
		t.Fatalf("repository default = %q", got)
	}
	if err := os.WriteFile(path, []byte(`{"server":"https://changed.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := settings.ServerDefault(""); got != "https://repo.example" {
		t.Fatalf("snapshot changed: %q", got)
	}
	if got := settings.ServerDefault("https://explicit.example"); got != "https://explicit.example" {
		t.Fatalf("explicit override = %q", got)
	}
	t.Setenv("GREPPLE_SERVER", "https://environment.example")
	if got := settings.ServerDefault(""); got != "https://environment.example" {
		t.Fatalf("environment override = %q", got)
	}
	t.Setenv("GREPPLE_SERVER", "")
	if err := os.WriteFile(filepath.Join(home, ".grepple", "config.json"), []byte(`{"server":"https://changed-user.example","token":"new-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertUserFallbackWithoutRepository(t, child, settings)
}

func assertUserFallbackWithoutRepository(t *testing.T, child string, settings *Config) {
	t.Helper()
	bypassed, err := LoadConfig(child, true)
	if err != nil {
		t.Fatal(err)
	}
	if bypassed.RepositoryPath != "" || bypassed.Repository.Server != "" || bypassed.ServerDefault("") != "https://changed-user.example" {
		t.Fatalf("bypassed repository: %+v", bypassed)
	}
	if settings.auth.Server != "https://user.example" {
		t.Fatalf("user server snapshot changed: %q", settings.auth.Server)
	}
	if settings.Repository.Token != "" || settings.Repository.RefreshToken != "" || settings.Repository.User != "" {
		t.Fatal("authentication token entered repository config")
	}
}

func TestLoadConfigEagerlySnapshotsUserSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	if err := os.WriteFile(path, []byte(`{"anchors":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(t.TempDir(), true); err == nil {
		t.Fatal("malformed user settings did not fail the complete load")
	}
	if err := os.WriteFile(path, []byte(`{"anchors":{},"context_guard":{"enabled":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadConfig(t.TempDir(), true)
	if err != nil || !settings.ContextGuardEnabled() {
		t.Fatalf("user settings were not eagerly loaded: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"anchors":{},"context_guard":{"enabled":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !settings.ContextGuardEnabled() {
		t.Fatal("configuration snapshot changed after file update")
	}
}

func TestLoadConfigIncludesAllSourcesWithoutLeakingCredentials(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GREPPLE_TOKEN", "")
	for _, dir := range []string{filepath.Join(root, ".grepple"), filepath.Join(home, ".grepple")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, ".grepple", "grepple.json"):  `{"server":"https://repository.example"}`,
		filepath.Join(home, ".grepple", "settings.json"): `{"anchors":{"default_provider":"native"}}`,
		filepath.Join(home, ".grepple", "grepple.json"):  `{"ask":{"model":"anthropic/claude","logs":{"retentionPeriod":"2d"}}}`,
		filepath.Join(home, ".grepple", "config.json"):   `{"server":"https://user.example","token":"sensitive-token"}`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := LoadConfig(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Repository.Server != "https://repository.example" || settings.User.Anchors.DefaultProvider != "native" || settings.Ask.Model != "anthropic/claude" || settings.Ask.LogRetention.Hours() != 48 || settings.AuthToken() != "sensitive-token" {
		t.Fatal("one configuration source was not loaded")
	}
	assertNoCredentialLeak(t, settings)
	t.Setenv("GREPPLE_TOKEN", "environment-token")
	if settings.AuthToken() != "environment-token" {
		t.Fatal("token environment override was ignored")
	}
}

func assertNoCredentialLeak(t *testing.T, settings *Config) {
	t.Helper()
	for _, rendered := range []string{fmt.Sprintf("%+v", settings), fmt.Sprintf("%#v", settings)} {
		if strings.Contains(rendered, "sensitive-token") {
			t.Fatal("formatted configuration leaked credentials")
		}
	}
	serialized, err := json.Marshal(settings)
	if err != nil || strings.Contains(string(serialized), "sensitive-token") {
		t.Fatalf("serialized configuration leaked credentials: %v", err)
	}
	if err := settings.SaveUserSettings(settings.User); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(settings.SettingsPath)
	if err != nil || strings.Contains(string(persisted), "sensitive-token") || strings.Contains(string(persisted), "https://repository.example") {
		t.Fatalf("user settings included other configuration sources: %v", err)
	}
}
