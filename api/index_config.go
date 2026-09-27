package api

import "github.com/greppleai/grepple/internal/wire"

// ValidateRepositoryIndexConfig validates repository identities and ref globs.
func ValidateRepositoryIndexConfig(config RepositoryIndexConfig) error {
	return wire.ValidateRepositoryIndexConfig(config)
}
