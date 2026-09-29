// Package config assembles repository, user, Ask, and protected authentication
// settings into a single invocation snapshot. Credential persistence remains
// owned by authstate and never belongs in repository-owned configuration.
package config

import (
	"fmt"
	"os"

	"github.com/greppleai/grepple/internal/authstate"
)

// Config contains all configuration read for one invocation. Authentication
// credentials are private so passing a Config cannot accidentally serialize
// tokens into output or repository settings. Login and refresh still update
// authstate independently; a new invocation reads the updated state.
type Config struct {
	Repository     Repository
	RepositoryPath string
	User           UserSettings
	SettingsPath   string
	Ask            AskPreferences

	auth authstate.Config
}

// LoadConfig reads every configuration source once, including user settings
// and legacy Ask preferences, and returns one validated snapshot. Disabling
// repository configuration does not disable user-owned settings or auth state.
func LoadConfig(start string, noRepositoryConfig bool) (*Config, error) {
	settings := &Config{auth: authstate.Load()}
	if !noRepositoryConfig {
		var err error
		settings.Repository, settings.RepositoryPath, err = loadRepository(start)
		if err != nil {
			return nil, err
		}
	}
	var err error
	settings.SettingsPath, err = userSettingsPath()
	if err != nil {
		return nil, err
	}
	settings.User, err = loadUserSettings(settings.SettingsPath)
	if err != nil {
		return nil, err
	}
	settings.Ask, err = loadAskPreferences()
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// String avoids including authentication state or provider command arguments
// in accidental diagnostic formatting.
func (settings *Config) String() string {
	if settings == nil {
		return "<nil config>"
	}
	return fmt.Sprintf("Config{RepositoryPath:%q, SettingsPath:%q}", settings.RepositoryPath, settings.SettingsPath)
}

// GoString keeps detailed diagnostics from printing protected auth state.
func (settings *Config) GoString() string { return settings.String() }

// UserSettings returns the already loaded user-owned settings.
func (settings *Config) UserSettings() (UserSettings, error) {
	return settings.User, nil
}

// ContextGuardEnabled reports the loaded context guard preference.
func (settings *Config) ContextGuardEnabled() bool {
	return settings.User.ContextGuard.Enabled == nil || *settings.User.ContextGuard.Enabled
}

// AuthToken returns the snapshot token, with an environment override. Token
// refresh and login still consult authstate directly to avoid stale credentials.
func (settings *Config) AuthToken() string {
	if token := os.Getenv("GREPPLE_TOKEN"); token != "" {
		return token
	}
	return settings.auth.Token
}

// ServerDefault applies explicit and environment overrides before persisted defaults.
func (settings *Config) ServerDefault(value string) string {
	if value != "" {
		return value
	}
	if value = os.Getenv("GREPPLE_SERVER"); value != "" {
		return value
	}
	if settings.Repository.Server != "" {
		return settings.Repository.Server
	}
	if settings.auth.Server != "" {
		return settings.auth.Server
	}
	return "http://127.0.0.1:8787"
}
