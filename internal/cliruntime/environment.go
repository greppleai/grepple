package cliruntime

import (
	"io"
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/authstate"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/storagepaths"
	"github.com/greppleai/grepple/internal/usersettings"
)

type invocationConfiguration struct {
	invocation            RepositoryInvocationOptions
	inlineOutputThreshold int
}

type invocationRepository struct {
	invocation RepositoryInvocationOptions
	notices    io.Writer
}

// NewConfiguration creates configuration backed by user and repository settings.
func NewConfiguration(invocation RepositoryInvocationOptions, inlineOutputThreshold int) Configuration {
	return invocationConfiguration{invocation: invocation, inlineOutputThreshold: inlineOutputThreshold}
}

func (configuration invocationConfiguration) ServerDefault(value string) string {
	if value != "" {
		return value
	}
	if value = os.Getenv("GREPPLE_SERVER"); value != "" {
		return value
	}
	if value = LoadConfig(configuration.invocation).Server; value != "" {
		return value
	}
	return "http://127.0.0.1:8787"
}
func (invocationConfiguration) ContextGuardEnabled() bool { return usersettings.ContextGuardEnabled() }
func (configuration invocationConfiguration) InlineOutputThreshold() int {
	return configuration.inlineOutputThreshold
}
func (invocationConfiguration) UserSettings() (usersettings.Config, error) {
	return usersettings.Load()
}

// NewRepository creates repository context for one CLI invocation.
func NewRepository(invocation RepositoryInvocationOptions, notices io.Writer) Repository {
	return invocationRepository{invocation: invocation, notices: notices}
}

func (repository invocationRepository) WorkingDirectory() string { return WorkingDirectory() }
func (invocationRepository) Current() string                     { return CurrentRepository() }
func (repository invocationRepository) ScopeOptions() (sourcedomain.Options, error) {
	working := WorkingDirectory()
	options := sourcedomain.Options{WorkingDirectory: working, IgnoreRoot: working, ProductionOnly: repository.invocation.ProductionOnly, Notices: repository.notices}
	config, path, err := LoadInvocationRepositoryConfig(repository.invocation)
	if err != nil {
		return options, err
	}
	if path != "" && !repository.invocation.NoConfigIgnore {
		options.IgnoreRoot = filepath.Dir(filepath.Dir(path))
		options.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
	}
	return options, nil
}
func (repository invocationRepository) AppendScopeFlags(parts []string) []string {
	return append(parts, RepositoryScopeFlags(repository.invocation)...)
}
func (repository invocationRepository) InvocationOptions() RepositoryInvocationOptions {
	return repository.invocation
}
func (repository invocationRepository) CacheDirectory() string {
	return storagepaths.Cache(WorkingDirectory())
}
func (repository invocationRepository) ConfigurationPath() (string, error) {
	_, path, err := LoadInvocationRepositoryConfig(repository.invocation)
	return path, err
}

// WorkingDirectory returns the current process directory or an empty string.
func WorkingDirectory() string {
	working, _ := os.Getwd()
	return working
}

// LoadInvocationRepositoryConfig loads repository config unless disabled for this invocation.
func LoadInvocationRepositoryConfig(invocation RepositoryInvocationOptions) (RepositoryConfig, string, error) {
	if invocation.NoRepositoryConfig {
		return RepositoryConfig{}, "", nil
	}
	return LoadRepositoryConfig(WorkingDirectory())
}

// LoadConfig loads user state and overlays repository-safe server selection.
func LoadConfig(invocation RepositoryInvocationOptions) authstate.Config {
	config := authstate.Load()
	if repository, _, err := LoadInvocationRepositoryConfig(invocation); err == nil && repository.Server != "" {
		config.Server = repository.Server
	}
	return config
}
