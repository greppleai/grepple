package grepplecli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type config struct {
	Server        string `json:"server,omitempty"`
	Token         string `json:"token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TokenExpiry   int64  `json:"token_expiry,omitempty"`   // unix seconds; 0 = unknown/never
	RefreshExpiry int64  `json:"refresh_expiry,omitempty"` // unix seconds; 0 = unknown/never
	User          string `json:"user,omitempty"`
}

// loadConfig merges ~/.grepple/config.json then ./grepple.json, overlaying only
// non-empty fields so the local file can override the server without dropping a
// token stored in the user config.
func loadConfig() config {
	var c config
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, ".grepple", "config.json"), filepath.Join(mustGetwd(), "grepple.json")} {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		var x config
		if json.Unmarshal(b, &x) != nil {
			continue
		}
		overlayConfig(&c, x)
	}
	return c
}

// overlayConfig copies x's non-empty fields over c, so a later config file
// overrides only what it actually sets.
func overlayConfig(c *config, x config) {
	if x.Server != "" {
		c.Server = x.Server
	}
	if x.Token != "" {
		c.Token = x.Token
	}
	if x.RefreshToken != "" {
		c.RefreshToken = x.RefreshToken
	}
	if x.TokenExpiry != 0 {
		c.TokenExpiry = x.TokenExpiry
	}
	if x.RefreshExpiry != 0 {
		c.RefreshExpiry = x.RefreshExpiry
	}
	if x.User != "" {
		c.User = x.User
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
