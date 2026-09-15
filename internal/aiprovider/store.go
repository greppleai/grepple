package aiprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const credentialsSchema = "grepple-ai-credentials-v1"

// CredentialsPathEnv overrides the user-owned AI credential file location.
const CredentialsPathEnv = "GREPPLE_AI_CREDENTIALS"

type credentialFile struct {
	Schema    string                     `json:"schema"`
	Providers map[string]json.RawMessage `json:"providers"`
}

// Store persists provider-specific opaque credentials outside repository configuration.
type Store struct {
	path string
}

// NewStore returns the user-owned provider credential store.
func NewStore() (*Store, error) {
	path := os.Getenv(CredentialsPathEnv)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".grepple", "ai-providers.json")
	}
	return &Store{path: path}, nil
}

// Path reports the credential file without exposing its contents.
func (s *Store) Path() string { return s.path }

// Load decodes one provider's credentials. found is false when it is not logged in.
func (s *Store) Load(provider string, target any) (found bool, err error) {
	file, err := s.loadFile()
	if err != nil {
		return false, err
	}
	raw, found := file.Providers[provider]
	if !found {
		return false, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, fmt.Errorf("decode %s credentials: %w", provider, err)
	}
	return true, nil
}

// Save atomically stores one provider's credentials with mode 0600.
func (s *Store) Save(provider string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.mutate(func(file *credentialFile) { file.Providers[provider] = raw })
}

// Delete removes one provider without affecting other provider sessions.
func (s *Store) Delete(provider string) error {
	return s.mutate(func(file *credentialFile) { delete(file.Providers, provider) })
}

func (s *Store) mutate(update func(*credentialFile)) error {
	release, err := acquireCredentialStoreLock(s.path)
	if err != nil {
		return err
	}
	defer release()
	file, err := s.loadFile()
	if err != nil {
		return err
	}
	update(&file)
	return s.saveFile(file)
}

func acquireCredentialStoreLock(path string) (func(), error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			return func() { _ = os.Remove(lockPath) }, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if information, err := os.Stat(lockPath); err == nil && time.Since(information.ModTime()) > 30*time.Second {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out locking AI credential store %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Store) loadFile() (credentialFile, error) {
	file := credentialFile{Schema: credentialsSchema, Providers: map[string]json.RawMessage{}}
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return file, fmt.Errorf("decode AI credentials: %w", err)
	}
	if file.Schema != credentialsSchema {
		return file, fmt.Errorf("unsupported AI credential schema %q", file.Schema)
	}
	if file.Providers == nil {
		file.Providers = map[string]json.RawMessage{}
	}
	return file, nil
}

func (s *Store) saveFile(file credentialFile) error {
	file.Schema = credentialsSchema
	if file.Providers == nil {
		file.Providers = map[string]json.RawMessage{}
	}
	content, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".ai-providers-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return err
	}
	return os.Chmod(s.path, 0o600)
}
