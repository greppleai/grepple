// Package authstate persists user-owned remote authentication credentials.
package authstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config is writable user authentication and server state.
type Config struct {
	Server        string `json:"server,omitempty"`
	Token         string `json:"token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TokenExpiry   int64  `json:"token_expiry,omitempty"`
	RefreshExpiry int64  `json:"refresh_expiry,omitempty"`
	User          string `json:"user,omitempty"`
}

// Path returns the writable per-user configuration path.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "config.json"), nil
}

// Load reads writable user state, returning an empty configuration when absent.
func Load() Config {
	var config Config
	if path, err := Path(); err == nil {
		if content, readErr := os.ReadFile(path); readErr == nil {
			_ = json.Unmarshal(content, &config)
		}
	}
	return config
}

// StoreLogin persists an access token and optional refresh token.
func StoreLogin(token, refreshToken string, expiresIn, refreshExpiresIn int, user string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	config := Load()
	config.Token, config.RefreshToken = token, refreshToken
	config.TokenExpiry, config.RefreshExpiry = expiryUnix(expiresIn), expiryUnix(refreshExpiresIn)
	if user != "" {
		config.User = user
	}
	return write(path, config)
}

// SaveToken stores one externally managed access token.
func SaveToken(token, user string) error { return StoreLogin(token, "", 0, 0, user) }

// Clear removes stored credentials while preserving unrelated user settings.
func Clear() error {
	path, err := Path()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config Config
	_ = json.Unmarshal(content, &config)
	config.Token, config.RefreshToken, config.User = "", "", ""
	config.TokenExpiry, config.RefreshExpiry = 0, 0
	return write(path, config)
}

// Token resolves the environment override or persisted access token.
func Token() string {
	if value := os.Getenv("GREPPLE_TOKEN"); value != "" {
		return value
	}
	return Load().Token
}

func expiryUnix(seconds int) int64 {
	if seconds <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(seconds) * time.Second).Unix()
}

func write(path string, config Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(content, '\n'), 0o600)
}
