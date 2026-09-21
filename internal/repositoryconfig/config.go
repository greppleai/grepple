// Package repositoryconfig loads and validates repository-owned Grepple settings.
package repositoryconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/api"
)

// Config is repository-owned configuration. Authentication fields are rejected.
type Config struct {
	Server        string                    `json:"server,omitempty"`
	Ignore        Ignore                    `json:"ignore,omitempty"`
	Output        Output                    `json:"output,omitempty"`
	Index         api.RepositoryIndexConfig `json:"index,omitempty"`
	Token         string                    `json:"token,omitempty"`
	RefreshToken  string                    `json:"refresh_token,omitempty"`
	TokenExpiry   int64                     `json:"token_expiry,omitempty"`
	RefreshExpiry int64                     `json:"refresh_expiry,omitempty"`
	User          string                    `json:"user,omitempty"`
}

// Ignore contains repository-relative ignored source patterns.
type Ignore struct {
	Paths []string `json:"paths,omitempty"`
}

// Output contains repository output policy.
type Output struct {
	SpillThresholdBytes int `json:"spillThresholdBytes,omitempty"`
}

// Load finds, decodes, and validates the nearest repository configuration.
func Load(start string) (Config, string, error) {
	path, found := Find(start)
	if !found {
		return Config{}, "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, path, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Config{}, path, fmt.Errorf("invalid repository config %s: trailing JSON content", path)
	}
	if config.Token != "" || config.RefreshToken != "" || config.TokenExpiry != 0 || config.RefreshExpiry != 0 || config.User != "" {
		return Config{}, path, fmt.Errorf("repository config %s must not contain authentication fields", path)
	}
	if config.Output.SpillThresholdBytes < 0 {
		return Config{}, path, fmt.Errorf("repository config %s output.spillThresholdBytes must be non-negative", path)
	}
	for index, pattern := range config.Ignore.Paths {
		value := strings.TrimPrefix(strings.TrimSpace(pattern), "!")
		clean := filepath.Clean(filepath.FromSlash(value))
		if value == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Config{}, path, fmt.Errorf("repository config %s ignore.paths[%d] must be repository-relative", path, index)
		}
	}
	if err := api.ValidateRepositoryIndexConfig(config.Index); err != nil {
		return Config{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	return config, path, nil
}

// Find returns the nearest ancestor grepple.json.
func Find(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(directory, "grepple.json")
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().IsRegular() {
			return path, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}
