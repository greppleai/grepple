// Package usersettings loads user-owned anchor and context settings.
package usersettings

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

// Config is user-owned Grepple configuration.
type Config struct {
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

// Load reads user settings.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read Grepple settings: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var settings Config
	if err := decoder.Decode(&settings); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

// ContextGuardEnabled reports whether output context tracking is enabled.
func ContextGuardEnabled() bool {
	settings, err := Load()
	if err != nil {
		return false
	}
	return settings.ContextGuard.Enabled == nil || *settings.ContextGuard.Enabled
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

// Path returns the user settings path.
func Path() (string, error) {
	if path := os.Getenv("GREPPLE_SETTINGS"); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "settings.json"), nil
}

// DisplayPath returns a user-facing settings path.
func DisplayPath(path string) string {
	if path == "" {
		return "$GREPPLE_SETTINGS or ~/.grepple/settings.json"
	}
	return path
}

// ResolveProvider resolves and validates an anchor provider.
func ResolveProvider(name string) (string, Provider, error) {
	settingsPath, _ := Path()
	settings, err := Load()
	if err != nil {
		return "", Provider{}, err
	}
	if name == "" {
		name = settings.Anchors.DefaultProvider
	}
	if name == "" {
		return "", Provider{}, fmt.Errorf("no default anchor provider configured in %s", DisplayPath(settingsPath))
	}
	provider, ok := settings.Anchors.Providers[name]
	if !ok {
		return "", Provider{}, fmt.Errorf("anchor provider %q is not configured in %s", name, DisplayPath(settingsPath))
	}
	if err := ValidateProvider(provider); err != nil {
		return "", Provider{}, fmt.Errorf("anchor provider %q: %w", name, err)
	}
	if provider.TimeoutMS == 0 {
		provider.TimeoutMS = defaultProviderTimeoutMS
	}
	return name, provider, nil
}

// ValidateProvider validates an anchor provider.
func ValidateProvider(provider Provider) error {
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

// Save writes settings atomically with private permissions.
func Save(path string, settings Config) error {
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
	if err := encoder.Encode(settings); err != nil {
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
	return os.Rename(temporaryPath, path)
}
