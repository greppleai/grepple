package cliruntime

import (
	"os"

	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/usersettings"
)

// ConfigurationServices adapts command configuration functions for tests and embedding.
type ConfigurationServices struct {
	ResolveServer     func(string) string
	ContextGuard      func() bool
	InlineOutputBytes func() int
	LoadUserSettings  func() (usersettings.Config, error)
}

func (services ConfigurationServices) ServerDefault(value string) string {
	if services.ResolveServer != nil {
		return services.ResolveServer(value)
	}
	return value
}
func (services ConfigurationServices) ContextGuardEnabled() bool {
	return services.ContextGuard != nil && services.ContextGuard()
}
func (services ConfigurationServices) InlineOutputThreshold() int {
	if services.InlineOutputBytes != nil {
		return services.InlineOutputBytes()
	}
	return 0
}
func (services ConfigurationServices) UserSettings() (usersettings.Config, error) {
	if services.LoadUserSettings != nil {
		return services.LoadUserSettings()
	}
	return usersettings.Config{}, nil
}

// RepositoryServices adapts command repository functions for tests and embedding.
type RepositoryServices struct {
	WorkingDirectoryFunc  func() string
	CurrentFunc           func() string
	ScopeOptionsFunc      func() (sourcedomain.Options, error)
	AppendScopeFlagsFunc  func([]string) []string
	Invocation            func() RepositoryInvocationOptions
	CacheDirectoryFunc    func() string
	ConfigurationPathFunc func() (string, error)
}

func (services RepositoryServices) WorkingDirectory() string {
	if services.WorkingDirectoryFunc != nil {
		return services.WorkingDirectoryFunc()
	}
	working, _ := os.Getwd()
	return working
}
func (services RepositoryServices) Current() string {
	if services.CurrentFunc != nil {
		return services.CurrentFunc()
	}
	return ""
}
func (services RepositoryServices) ScopeOptions() (sourcedomain.Options, error) {
	if services.ScopeOptionsFunc != nil {
		return services.ScopeOptionsFunc()
	}
	working := services.WorkingDirectory()
	return sourcedomain.Options{WorkingDirectory: working, IgnoreRoot: working}, nil
}
func (services RepositoryServices) AppendScopeFlags(parts []string) []string {
	if services.AppendScopeFlagsFunc != nil {
		return services.AppendScopeFlagsFunc(parts)
	}
	return parts
}
func (services RepositoryServices) InvocationOptions() RepositoryInvocationOptions {
	if services.Invocation != nil {
		return services.Invocation()
	}
	return RepositoryInvocationOptions{}
}
func (services RepositoryServices) CacheDirectory() string {
	if services.CacheDirectoryFunc != nil {
		return services.CacheDirectoryFunc()
	}
	return ""
}
func (services RepositoryServices) ConfigurationPath() (string, error) {
	if services.ConfigurationPathFunc != nil {
		return services.ConfigurationPathFunc()
	}
	return "", nil
}
