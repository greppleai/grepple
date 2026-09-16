package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/greppleai/grepple/api"
)

type config struct {
	Server        string `json:"server,omitempty"`
	Token         string `json:"token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TokenExpiry   int64  `json:"token_expiry,omitempty"`   // unix seconds; 0 = unknown/never
	RefreshExpiry int64  `json:"refresh_expiry,omitempty"` // unix seconds; 0 = unknown/never
	User          string `json:"user,omitempty"`
}

type userPreferences struct {
	Ask userAskPreferences `json:"ask,omitempty"`
}

type userAskPreferences struct {
	Model string                `json:"model,omitempty"`
	Logs  userAskLogPreferences `json:"logs,omitempty"`
}

type userAskLogPreferences struct {
	Enabled         *bool  `json:"enabled,omitempty"`
	RetentionPeriod string `json:"retentionPeriod,omitempty"`
}

type configuredAskPreferences struct {
	Model        string
	LogsEnabled  bool
	LogRetention time.Duration
}

type repositoryConfig struct {
	Server        string                    `json:"server,omitempty"`
	Ignore        repositoryIgnoreConfig    `json:"ignore,omitempty"`
	Output        repositoryOutputConfig    `json:"output,omitempty"`
	Index         api.RepositoryIndexConfig `json:"index,omitempty"`
	Token         string                    `json:"token,omitempty"`
	RefreshToken  string                    `json:"refresh_token,omitempty"`
	TokenExpiry   int64                     `json:"token_expiry,omitempty"`
	RefreshExpiry int64                     `json:"refresh_expiry,omitempty"`
	User          string                    `json:"user,omitempty"`
}

type repositoryIgnoreConfig struct {
	Paths []string `json:"paths,omitempty"`
}

type repositoryOutputConfig struct {
	SpillThresholdBytes int `json:"spillThresholdBytes,omitempty"`
}

// loadConfig merges the writable user config with repository-safe fields from the
// nearest ancestor grepple.json. Repository files can select a server but can never
// provide or override authentication state.
func loadConfig() config {
	var c config
	if path, err := userConfigPath(); err == nil {
		if content, readErr := os.ReadFile(path); readErr == nil {
			_ = json.Unmarshal(content, &c)
		}
	}
	if repository, _, err := loadRepositoryConfig(); err == nil && repository.Server != "" {
		c.Server = repository.Server
	}
	return c
}

func loadRepositoryConfig() (repositoryConfig, string, error) {
	if activeRepositoryOptions.disabled {
		return repositoryConfig{}, "", nil
	}
	path, found := findRepositoryConfig(mustGetwd())
	if !found {
		return repositoryConfig{}, "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return repositoryConfig{}, path, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var config repositoryConfig
	if err := decoder.Decode(&config); err != nil {
		return repositoryConfig{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return repositoryConfig{}, path, fmt.Errorf("invalid repository config %s: trailing JSON content", path)
	}
	if config.Token != "" || config.RefreshToken != "" || config.TokenExpiry != 0 || config.RefreshExpiry != 0 || config.User != "" {
		return repositoryConfig{}, path, fmt.Errorf("repository config %s must not contain authentication fields", path)
	}
	if config.Output.SpillThresholdBytes < 0 {
		return repositoryConfig{}, path, fmt.Errorf("repository config %s output.spillThresholdBytes must be non-negative", path)
	}
	for index, pattern := range config.Ignore.Paths {
		value := strings.TrimPrefix(strings.TrimSpace(pattern), "!")
		clean := filepath.Clean(filepath.FromSlash(value))
		if value == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return repositoryConfig{}, path, fmt.Errorf("repository config %s ignore.paths[%d] must be repository-relative", path, index)
		}
	}
	if err := api.ValidateRepositoryIndexConfig(config.Index); err != nil {
		return repositoryConfig{}, path, fmt.Errorf("invalid repository config %s: %w", path, err)
	}
	return config, path, nil
}

func findRepositoryConfig(start string) (string, bool) {
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

// userConfigPath is the writable per-user config (~/.grepple/config.json), where
// `grepple login` stores the token.
func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "config.json"), nil
}

// userPreferencesPath is the non-secret per-user configuration file.
func userPreferencesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "grepple.json"), nil
}

func loadConfiguredAskPreferences() (configuredAskPreferences, error) {
	configured := configuredAskPreferences{LogsEnabled: true, LogRetention: 7 * 24 * time.Hour}
	path, err := userPreferencesPath()
	if err != nil {
		return configured, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return configured, nil
	}
	if err != nil {
		return configured, fmt.Errorf("read user configuration %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	var preferences userPreferences
	if err := decoder.Decode(&preferences); err != nil {
		return configured, fmt.Errorf("invalid user configuration %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return configured, fmt.Errorf("invalid user configuration %s: trailing JSON content", path)
	}
	configured.Model = strings.TrimSpace(preferences.Ask.Model)
	if preferences.Ask.Logs.Enabled != nil {
		configured.LogsEnabled = *preferences.Ask.Logs.Enabled
	}
	if retention := strings.TrimSpace(preferences.Ask.Logs.RetentionPeriod); retention != "" {
		configured.LogRetention, err = parseAskLogRetention(retention)
		if err != nil {
			return configured, fmt.Errorf("invalid user configuration %s ask.logs.retentionPeriod: %w", path, err)
		}
	}
	return configured, nil
}

func parseAskLogRetention(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		maximumDays := int64((1<<63 - 1) / int64(24*time.Hour))
		if err != nil || days < 1 || int64(days) > maximumDays {
			return 0, fmt.Errorf("must be a positive duration such as 7d or 168h")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("must be a positive duration such as 7d or 168h")
	}
	return duration, nil
}

// storeLogin persists a token set (access token, optional refresh token, and
// their lifetimes) into the user config, preserving unrelated fields. expiresIn
// and refreshExpiresIn are seconds-from-now (0 = unknown, stored as no expiry).
// The file is 0600 since it holds credentials.
func storeLogin(token, refreshToken string, expiresIn, refreshExpiresIn int, user string) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	var c config
	if b, e := os.ReadFile(path); e == nil {
		_ = json.Unmarshal(b, &c)
	}
	c.Token = token
	c.RefreshToken = refreshToken
	c.TokenExpiry = expiryUnix(expiresIn)
	c.RefreshExpiry = expiryUnix(refreshExpiresIn)
	if user != "" {
		c.User = user
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// saveToken stores just an access token (no refresh/expiry) — used where a
// long-lived or externally managed token is provided.
func saveToken(token, user string) error {
	return storeLogin(token, "", 0, 0, user)
}

// expiryUnix converts a seconds-from-now lifetime into an absolute unix
// timestamp, or 0 when the lifetime is unknown/non-expiring.
func expiryUnix(seconds int) int64 {
	if seconds <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(seconds) * time.Second).Unix()
}

// clearToken removes the stored token/user from the user config.
func clearToken() error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	var c config
	b, e := os.ReadFile(path)
	if e != nil {
		return nil // nothing stored
	}
	_ = json.Unmarshal(b, &c)
	c.Token = ""
	c.RefreshToken = ""
	c.TokenExpiry = 0
	c.RefreshExpiry = 0
	c.User = ""
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o600)
}

// configuredToken resolves the GitHub token: GREPPLE_TOKEN env wins, else the
// stored config token.
func configuredToken() string {
	if v := os.Getenv("GREPPLE_TOKEN"); v != "" {
		return v
	}
	return loadConfig().Token
}

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
