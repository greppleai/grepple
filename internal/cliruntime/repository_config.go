package cliruntime

import "github.com/greppleai/grepple/internal/config"

// RepositoryConfig is the repository-owned configuration (owned by config).
type RepositoryConfig = config.Repository

// RepositoryIgnore contains repository-relative ignored source patterns.
type RepositoryIgnore = config.Ignore

// RepositoryOutput contains repository output policy.
type RepositoryOutput = config.Output

// LoadRepositoryConfig returns the repository portion of a fully loaded config.
func LoadRepositoryConfig(start string) (RepositoryConfig, string, error) {
	settings, err := config.LoadConfig(start, false)
	if err != nil {
		return RepositoryConfig{}, "", err
	}
	return settings.Repository, settings.RepositoryPath, nil
}
