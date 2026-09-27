package cliruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/wire"
)

// RepositoryConfig is repository-owned configuration. Authentication fields are rejected.
type RepositoryConfig struct {
	Server        string                    `json:"server,omitempty"`
	Ignore        RepositoryIgnore          `json:"ignore,omitempty"`
	Output        RepositoryOutput          `json:"output,omitempty"`
	Index         wire.RepositoryIndexConfig `json:"index,omitempty"`
	Token         string                    `json:"token,omitempty"`
	RefreshToken  string                    `json:"refresh_token,omitempty"`
	TokenExpiry   int64                     `json:"token_expiry,omitempty"`
	RefreshExpiry int64                     `json:"refresh_expiry,omitempty"`
	User          string                    `json:"user,omitempty"`
}

// RepositoryIgnore contains repository-relative ignored source patterns.
type RepositoryIgnore struct {
	Paths []string `json:"paths,omitempty"`
}

// RepositoryOutput contains repository output policy.
type RepositoryOutput struct {
	SpillThresholdBytes int `json:"spillThresholdBytes,omitempty"`
}

// LoadRepositoryConfig finds, decodes, and validates the nearest repository configuration.
func LoadRepositoryConfig(start string) (RepositoryConfig, string, error) {
	path, found := FindRepositoryConfig(start)
	if !found {
		return RepositoryConfig{}, "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return RepositoryConfig{}, path, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var config RepositoryConfig
	if err := decoder.Decode(&config); err != nil {
		return RepositoryConfig{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RepositoryConfig{}, path, fmt.Errorf("invalid repository config %s: trailing JSON content", path)
	}
	if config.Token != "" || config.RefreshToken != "" || config.TokenExpiry != 0 || config.RefreshExpiry != 0 || config.User != "" {
		return RepositoryConfig{}, path, fmt.Errorf("repository config %s must not contain authentication fields", path)
	}
	if config.Output.SpillThresholdBytes < 0 {
		return RepositoryConfig{}, path, fmt.Errorf("repository config %s output.spillThresholdBytes must be non-negative", path)
	}
	for index, pattern := range config.Ignore.Paths {
		value := strings.TrimPrefix(strings.TrimSpace(pattern), "!")
		clean := filepath.Clean(filepath.FromSlash(value))
		if value == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return RepositoryConfig{}, path, fmt.Errorf("repository config %s ignore.paths[%d] must be repository-relative", path, index)
		}
	}
	if err := wire.ValidateRepositoryIndexConfig(config.Index); err != nil {
		return RepositoryConfig{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	return config, path, nil
}

// FindRepositoryConfig returns the nearest ancestor grepple.json.
func FindRepositoryConfig(start string) (string, bool) {
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
