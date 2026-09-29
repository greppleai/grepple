package cliruntime

import (
	"io"
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/authstate"
	"github.com/greppleai/grepple/internal/config"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/storagepaths"
)

type invocationConfiguration struct {
	invocation            RepositoryInvocationOptions
	inlineOutputThreshold int
	config                *config.Config
}

type invocationRepository struct {
	invocation RepositoryInvocationOptions
	notices    io.Writer
	config     *config.Config
}

// NewConfiguration creates configuration backed by user and repository settings.
func NewConfiguration(invocation RepositoryInvocationOptions, inlineOutputThreshold int) Configuration {
	return newConfigurationWithConfig(invocation, inlineOutputThreshold, nil)
}

func newConfigurationWithConfig(invocation RepositoryInvocationOptions, threshold int, settings *config.Config) Configuration {
	return invocationConfiguration{invocation: invocation, inlineOutputThreshold: threshold, config: settings}
}

func (configuration invocationConfiguration) ServerDefault(value string) string {
	if configuration.config != nil {
		return configuration.config.ServerDefault(value)
	}
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
func (configuration invocationConfiguration) ContextGuardEnabled() bool {
	if configuration.config != nil {
		return configuration.config.ContextGuardEnabled()
	}
	settings, err := config.LoadConfig(WorkingDirectory(), configuration.invocation.NoRepositoryConfig)
	return err == nil && settings.ContextGuardEnabled()
}
func (configuration invocationConfiguration) InlineOutputThreshold() int {
	return configuration.inlineOutputThreshold
}
func (configuration invocationConfiguration) UserSettings() (config.UserSettings, error) {
	if configuration.config != nil {
		return configuration.config.UserSettings()
	}
	settings, err := config.LoadConfig(WorkingDirectory(), configuration.invocation.NoRepositoryConfig)
	if err != nil {
		return config.UserSettings{}, err
	}
	return settings.UserSettings()
}

func (configuration invocationConfiguration) AskPreferences() (config.AskPreferences, error) {
	if configuration.config != nil {
		return configuration.config.Ask, nil
	}
	settings, err := config.LoadConfig(WorkingDirectory(), configuration.invocation.NoRepositoryConfig)
	if err != nil {
		return config.AskPreferences{}, err
	}
	return settings.Ask, nil
}

// NewRepository creates repository context for one CLI invocation.
func NewRepository(invocation RepositoryInvocationOptions, notices io.Writer) Repository {
	return newRepositoryWithConfig(invocation, notices, nil)
}

func newRepositoryWithConfig(invocation RepositoryInvocationOptions, notices io.Writer, settings *config.Config) Repository {
	return invocationRepository{invocation: invocation, notices: notices, config: settings}
}

func (repository invocationRepository) WorkingDirectory() string { return WorkingDirectory() }
func (invocationRepository) Current() string                     { return CurrentRepository() }
func (repository invocationRepository) ScopeOptions() (sourcedomain.Options, error) {
	working := WorkingDirectory()
	options := sourcedomain.Options{WorkingDirectory: working, IgnoreRoot: working, ProductionOnly: repository.invocation.ProductionOnly, Notices: repository.notices}
	var settings RepositoryConfig
	var path string
	var err error
	if repository.config != nil {
		settings, path = repository.config.Repository, repository.config.RepositoryPath
	} else {
		settings, path, err = LoadInvocationRepositoryConfig(repository.invocation)
	}
	if err != nil {
		return options, err
	}
	if path != "" && !repository.invocation.NoConfigIgnore {
		options.IgnoreRoot = filepath.Dir(filepath.Dir(path))
		options.IgnorePaths = append([]string(nil), settings.Ignore.Paths...)
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
	if repository.config != nil {
		return repository.config.RepositoryPath, nil
	}
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
