package config

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

// Repository is shareable repository-owned configuration; authentication fields are rejected.
type Repository struct {
	Server        string                     `json:"server,omitempty"`
	Ignore        Ignore                     `json:"ignore,omitempty"`
	Output        Output                     `json:"output,omitempty"`
	Index         wire.RepositoryIndexConfig `json:"index,omitempty"`
	Token         string                     `json:"token,omitempty"`
	RefreshToken  string                     `json:"refresh_token,omitempty"`
	TokenExpiry   int64                      `json:"token_expiry,omitempty"`
	RefreshExpiry int64                      `json:"refresh_expiry,omitempty"`
	User          string                     `json:"user,omitempty"`
}

// Ignore contains repository-relative ignored source patterns.
type Ignore struct {
	Paths []string `json:"paths,omitempty"`
}

// Output contains repository output policy.
type Output struct {
	SpillThresholdBytes int `json:"spillThresholdBytes,omitempty"`
}

// loadRepository finds, decodes, and validates the nearest repository configuration.
func loadRepository(start string) (Repository, string, error) {
	path, found := findRepository(start)
	if !found {
		return Repository{}, "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Repository{}, path, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var settings Repository
	if err := decoder.Decode(&settings); err != nil {
		return Repository{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Repository{}, path, fmt.Errorf("invalid repository config %s: trailing JSON content", path)
	}
	if settings.Token != "" || settings.RefreshToken != "" || settings.TokenExpiry != 0 || settings.RefreshExpiry != 0 || settings.User != "" {
		return Repository{}, path, fmt.Errorf("repository config %s must not contain authentication fields", path)
	}
	if settings.Output.SpillThresholdBytes < 0 {
		return Repository{}, path, fmt.Errorf("repository config %s output.spillThresholdBytes must be non-negative", path)
	}
	for index, pattern := range settings.Ignore.Paths {
		value := strings.TrimPrefix(strings.TrimSpace(pattern), "!")
		clean := filepath.Clean(filepath.FromSlash(value))
		if value == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Repository{}, path, fmt.Errorf("repository config %s ignore.paths[%d] must be repository-relative", path, index)
		}
	}
	if err := wire.ValidateRepositoryIndexConfig(settings.Index); err != nil {
		return Repository{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	return settings, path, nil
}

// findRepository returns the nearest ancestor .grepple/grepple.json.
func findRepository(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(directory, ".grepple", "grepple.json")
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
