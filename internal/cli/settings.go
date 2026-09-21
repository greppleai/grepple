package cli

import "github.com/greppleai/grepple/internal/usersettings"

type userSettings = usersettings.Config
type contextGuardSettings = usersettings.ContextGuard
type anchorSettings = usersettings.Anchors
type anchorProviderSettings = usersettings.Provider

func loadUserSettings() (userSettings, error) { return usersettings.Load() }
func contextGuardEnabled() bool               { return usersettings.ContextGuardEnabled() }
func userSettingsPath() (string, error)       { return usersettings.Path() }
func displaySettingsPath(path string) string  { return usersettings.DisplayPath(path) }
func resolveAnchorProvider(name string) (string, anchorProviderSettings, error) {
	return usersettings.ResolveProvider(name)
}
func validateAnchorProviderSettings(provider anchorProviderSettings) error {
	return usersettings.ValidateProvider(provider)
}
