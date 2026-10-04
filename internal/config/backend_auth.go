package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// BackendCredentials are Grepple server credentials, not AI-provider keys.
// A copy is exposed only to consumers implementing backend token refresh.
type BackendCredentials struct {
	AuthServer    string
	Token         string
	RefreshToken  string
	TokenExpiry   int64
	RefreshExpiry int64
	User          string
}

type backendAuthState struct {
	Server        string `json:"server,omitempty"`
	AuthServer    string `json:"auth_server,omitempty"`
	Token         string `json:"token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	TokenExpiry   int64  `json:"token_expiry,omitempty"`
	RefreshExpiry int64  `json:"refresh_expiry,omitempty"`
	User          string `json:"user,omitempty"`
}

// BackendAuthPath returns the existing protected per-user backend login path.
func (*Config) BackendAuthPath() (string, error) { return backendAuthPath() }

func backendAuthPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "config.json"), nil
}

func loadBackendAuthState() backendAuthState {
	var state backendAuthState
	if path, err := backendAuthPath(); err == nil {
		if content, readErr := os.ReadFile(path); readErr == nil {
			_ = json.Unmarshal(content, &state)
		}
	}
	return state
}

// BackendCredentials returns a copy of the credentials loaded for this invocation.
func (settings *Config) BackendCredentials() BackendCredentials {
	return BackendCredentials{
		AuthServer: settings.auth.AuthServer, Token: settings.auth.Token, RefreshToken: settings.auth.RefreshToken,
		TokenExpiry: settings.auth.TokenExpiry, RefreshExpiry: settings.auth.RefreshExpiry,
		User: settings.auth.User,
	}
}

// StoreBackendLogin updates backend credentials without altering the server.
// Re-read the protected file so refreshes preserve changes made after loading.
func (settings *Config) StoreBackendLogin(token, refreshToken string, expiresIn, refreshExpiresIn int, user string, server string) error {
	path, err := backendAuthPath()
	if err != nil {
		return err
	}
	state := loadBackendAuthState()
	state.AuthServer = server
	state.Token, state.RefreshToken = token, refreshToken
	state.TokenExpiry, state.RefreshExpiry = backendExpiryUnix(expiresIn), backendExpiryUnix(refreshExpiresIn)
	if user != "" {
		state.User = user
	}
	if err := writeBackendAuth(path, state); err != nil {
		return err
	}
	settings.auth = state
	return nil
}

// ClearBackendLogin removes stored backend credentials while preserving the server.
func (settings *Config) ClearBackendLogin() error {
	path, err := backendAuthPath()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		settings.auth = backendAuthState{}
		return nil
	}
	if err != nil {
		return err
	}
	var state backendAuthState
	_ = json.Unmarshal(content, &state)
	state.AuthServer = ""
	state.Token, state.RefreshToken, state.User = "", "", ""
	state.TokenExpiry, state.RefreshExpiry = 0, 0
	if err := writeBackendAuth(path, state); err != nil {
		return err
	}
	settings.auth = state
	return nil
}

func backendExpiryUnix(seconds int) int64 {
	if seconds <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(seconds) * time.Second).Unix()
}

func writeBackendAuth(path string, state backendAuthState) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".backend-auth-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}
