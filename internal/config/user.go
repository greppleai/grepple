package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
)

const (
	defaultProviderTimeoutMS = 5000
	maxProviderTimeoutMS     = 60000
)

// UserSettings is user-owned Grepple configuration.
type UserSettings struct {
	Anchors   Anchors    `json:"anchors"`
	Telemetry *Telemetry `json:"telemetry,omitempty"`
}

// Telemetry controls user-owned, opt-in CLI traces; it cannot be set by repository config.
type Telemetry struct {
	Enabled          bool     `json:"enabled"`
	TraceEndpoint    string   `json:"trace_endpoint,omitempty"`
	TraceSampleRatio *float64 `json:"trace_sample_ratio,omitempty"`
}

// SampleRatio defaults to full sampling for an explicitly enabled local diagnostic session.
func (settings Telemetry) SampleRatio() float64 {
	if settings.TraceSampleRatio == nil {
		return 1
	}
	return *settings.TraceSampleRatio
}

// Validate rejects remote plaintext exporters and URLs capable of carrying credentials.
func (settings Telemetry) Validate() error {
	if ratio := settings.SampleRatio(); math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > 1 {
		return fmt.Errorf("telemetry trace_sample_ratio must be between 0 and 1")
	}
	if !settings.Enabled && settings.TraceEndpoint == "" {
		return nil
	}
	endpoint, err := url.Parse(settings.TraceEndpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawFragment != "" || endpoint.Path != "/v1/traces" || endpoint.RawPath != "" {
		return fmt.Errorf("telemetry trace_endpoint requires an exact /v1/traces URL without credentials, query or fragment")
	}
	if endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return fmt.Errorf("telemetry trace_endpoint requires HTTPS or explicit loopback HTTP")
	}
	host := endpoint.Hostname()
	if host == "" || (endpoint.Scheme == "http" && host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
		return fmt.Errorf("telemetry trace_endpoint requires HTTPS or explicit loopback HTTP")
	}
	return nil
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
	if settings.Telemetry != nil {
		if err := settings.Telemetry.Validate(); err != nil {
			return UserSettings{}, err
		}
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
	if user.Telemetry != nil {
		if err := user.Telemetry.Validate(); err != nil {
			return err
		}
	}
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
