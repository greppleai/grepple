package cli

import (
	"os"

	"github.com/greppleai/grepple/internal/authstate"
	"github.com/greppleai/grepple/internal/repositoryconfig"
)

type config = authstate.Config

type repositoryConfig = repositoryconfig.Config
type repositoryIgnoreConfig = repositoryconfig.Ignore
type repositoryOutputConfig = repositoryconfig.Output

// loadConfig merges the writable user config with repository-safe fields from the
// nearest ancestor grepple.json. Repository files can select a server but can never
// provide or override authentication state.
func loadConfig() config {
	c := authstate.Load()
	if repository, _, err := loadRepositoryConfig(); err == nil && repository.Server != "" {
		c.Server = repository.Server
	}
	return c
}

func loadRepositoryConfig() (repositoryConfig, string, error) {
	if activeRepositoryOptions.disabled {
		return repositoryConfig{}, "", nil
	}
	return repositoryconfig.Load(mustGetwd())
}

func findRepositoryConfig(start string) (string, bool) { return repositoryconfig.Find(start) }

func mustGetwd() string {
	x, _ := os.Getwd()
	return x
}

func configuredServer(flag string) (string, bool) {
	if flag != "" {
		return flag, true
	}
	if value := os.Getenv("GREPPLE_SERVER"); value != "" {
		return value, true
	}
	if value := loadConfig().Server; value != "" {
		return value, true
	}
	return "", false
}

func serverDefault(flag string) string {
	if server, configured := configuredServer(flag); configured {
		return server
	}
	return "http://127.0.0.1:8787"
}
