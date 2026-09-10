package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultAnchorProviderTimeoutMS = 5000
	maxAnchorProviderTimeoutMS     = 60000
)

type userSettings struct {
	Anchors anchorSettings `json:"anchors"`
}

type anchorSettings struct {
	EnabledByDefault bool                              `json:"enabled_by_default"`
	DefaultProvider  string                            `json:"default_provider"`
	Providers        map[string]anchorProviderSettings `json:"providers"`
}

type anchorProviderSettings struct {
	Command   []string `json:"command"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

func loadUserSettings() (userSettings, error) {
	path, err := userSettingsPath()
	if err != nil {
		return userSettings{}, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return userSettings{}, nil
	}
	if err != nil {
		return userSettings{}, fmt.Errorf("read Grepple settings: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var settings userSettings
	if err := decoder.Decode(&settings); err != nil {
		return userSettings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return userSettings{}, fmt.Errorf("parse %s: %w", path, err)
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

func resolveAnchorProvider(name string) (string, anchorProviderSettings, error) {
	settings, err := loadUserSettings()
	if err != nil {
		return "", anchorProviderSettings{}, err
	}
	if name == "" {
		name = settings.Anchors.DefaultProvider
	}
	if name == "" {
		return "", anchorProviderSettings{}, fmt.Errorf("no default anchor provider configured in ~/.grepple/settings.json")
	}
	provider, ok := settings.Anchors.Providers[name]
	if !ok {
		return "", anchorProviderSettings{}, fmt.Errorf("anchor provider %q is not configured in ~/.grepple/settings.json", name)
	}
	if err := validateAnchorProviderSettings(provider); err != nil {
		return "", anchorProviderSettings{}, fmt.Errorf("anchor provider %q: %w", name, err)
	}
	if provider.TimeoutMS == 0 {
		provider.TimeoutMS = defaultAnchorProviderTimeoutMS
	}
	return name, provider, nil
}

func validateAnchorProviderSettings(provider anchorProviderSettings) error {
	if len(provider.Command) == 0 || provider.Command[0] == "" {
		return fmt.Errorf("command must contain an executable")
	}
	if !filepath.IsAbs(provider.Command[0]) {
		return fmt.Errorf("command executable must be an absolute path")
	}
	if provider.TimeoutMS < 0 || provider.TimeoutMS > maxAnchorProviderTimeoutMS {
		return fmt.Errorf("timeout_ms must be between 0 and %d", maxAnchorProviderTimeoutMS)
	}
	return nil
}
