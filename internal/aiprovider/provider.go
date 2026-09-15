// Package aiprovider defines provider-agnostic authentication and model construction for grepple ask.
package aiprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"

	"charm.land/fantasy"
)

// LoginOptions controls an interactive provider login.
type LoginOptions struct {
	NoBrowser bool
	Output    io.Writer
}

// Provider owns one model service's authentication and Fantasy model adapter.
type Provider interface {
	Name() string
	DefaultModel() string
	Login(context.Context, LoginOptions) error
	Logout() error
	LoggedIn() (bool, error)
	LanguageModel(context.Context, string) (fantasy.LanguageModel, error)
}

// Registry resolves providers without exposing provider-specific logic to commands.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry constructs the built-in provider registry.
func NewRegistry(store *Store, client *http.Client) *Registry {
	if client == nil {
		client = http.DefaultClient
	}
	codex := newCodexProvider(store, client)
	return &Registry{providers: map[string]Provider{codex.Name(): codex}}
}

// Provider resolves a configured provider by stable name.
func (r *Registry) Provider(name string) (Provider, error) {
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown AI provider %q (available: %v)", name, r.Names())
	}
	return provider, nil
}

// Names lists provider names deterministically.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
