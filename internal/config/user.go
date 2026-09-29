package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultProviderTimeoutMS = 5000
	maxProviderTimeoutMS     = 60000
)

// UserSettings is user-owned Grepple configuration.
type UserSettings struct {
	Anchors      Anchors      `json:"anchors"`
	ContextGuard ContextGuard `json:"context_guard,omitempty"`
}

// ContextGuard controls rendered source deduplication.
type ContextGuard struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// Anchors configures edit-anchor providers.
type Anchors struct {
	EnabledByDefault bool                `json:"enabled_by_default"`
	DefaultProvider  string              `json:"default_provider"`
	Providers        map[string]Provider `json:"providers"`
}

// Provider configures one edit-anchor executable.
type Provider struct {
	Command   []string `json:"command"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

// loadUserSettings reads user settings from the selected path.
func loadUserSettings(path string) (UserSettings, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return UserSettings{}, nil
	}
	if err != nil {
		return UserSettings{}, fmt.Errorf("read Grepple settings: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var settings UserSettings
	if err := decoder.Decode(&settings); err != nil {
		return UserSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return UserSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("multiple JSON values")
}

// userSettingsPath returns the user settings path.
func userSettingsPath() (string, error) {
	if path := os.Getenv("GREPPLE_SETTINGS"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "settings.json"), nil
}

// displayUserSettingsPath returns a user-facing settings path.
func displayUserSettingsPath(path string) string {
	if path == "" {
		return "$GREPPLE_SETTINGS or ~/.grepple/settings.json"
	}
	return path
}

// ResolveProvider resolves and validates an anchor provider from this snapshot.
func (settings *Config) ResolveProvider(name string) (string, Provider, error) {
	path := settings.SettingsPath
	if name == "" {
		name = settings.User.Anchors.DefaultProvider
	}
	if name == "" {
		return "", Provider{}, fmt.Errorf("no default anchor provider configured in %s", displayUserSettingsPath(path))
	}
	provider, ok := settings.User.Anchors.Providers[name]
	if !ok {
		return "", Provider{}, fmt.Errorf("anchor provider %q is not configured in %s", name, displayUserSettingsPath(path))
	}
	if err := provider.Validate(); err != nil {
		return "", Provider{}, fmt.Errorf("anchor provider %q: %w", name, err)
	}
	if provider.TimeoutMS == 0 {
		provider.TimeoutMS = defaultProviderTimeoutMS
	}
	return name, provider, nil
}

// Validate checks an anchor provider.
func (provider Provider) Validate() error {
	if len(provider.Command) == 0 || provider.Command[0] == "" {
		return fmt.Errorf("command must contain an executable")
	}
	if !filepath.IsAbs(provider.Command[0]) {
		return fmt.Errorf("command executable must be an absolute path")
	}
	if provider.TimeoutMS < 0 || provider.TimeoutMS > maxProviderTimeoutMS {
		return fmt.Errorf("timeout_ms must be between 0 and %d", maxProviderTimeoutMS)
	}
	return nil
}

// SaveUserSettings writes the new user settings atomically with private permissions.
func (settings *Config) SaveUserSettings(user UserSettings) error {
	path := settings.SettingsPath
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(user); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	settings.User = user
	return nil
}
