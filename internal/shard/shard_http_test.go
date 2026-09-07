package shard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grepple/internal/api"
	"grepple/internal/repository"
)

func TestFiberHTTPRoutes(t *testing.T) {
	root := t.TempDir()
	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	app := (&shardImpl{registry: registry, root: root}).Handler()

	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/health", "", http.StatusOK},
		{http.MethodGet, "/ready", "", http.StatusServiceUnavailable},
		{http.MethodGet, "/", "", http.StatusServiceUnavailable},
		{http.MethodPost, "/search", "not-json", http.StatusBadRequest},
		{http.MethodGet, "/raw", "", http.StatusBadRequest},
		{http.MethodGet, "/tree", "", http.StatusBadRequest},
		{http.MethodGet, "/missing", "", http.StatusNotFound},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response, err := app.Test(request)
		if err != nil {
			t.Fatalf("%s %s: %v", test.method, test.path, err)
		}
		if response.StatusCode != test.status {
			t.Errorf("%s %s status=%d want %d", test.method, test.path, response.StatusCode, test.status)
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Errorf("%s %s returned invalid JSON: %v", test.method, test.path, err)
		}
		response.Body.Close()
	}

	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\n\nfunc hello() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"query": "hello", "globs": []string{path}})
	request := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(string(body)))
	request.Header.Set("content-type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("search status=%d", response.StatusCode)
	}
	var result api.SearchResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Path == "" {
		t.Fatalf("search results=%#v", result.Results)
	}
}

func TestSearchIsConfinedToRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(parent, "secret.go")
	if err := os.WriteFile(secret, []byte("package secret\n// hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	state := &shardImpl{registry: registry, root: root}

	for _, glob := range []string{secret, filepath.Join(root, "..", "*.go"), "**/*.go"} {
		query := "hello"
		response, err := state.runSearchRequest(api.SearchRequest{Query: &query, Globs: []string{glob}}, "")
		if err != nil {
			t.Fatalf("glob %q: %v", glob, err)
		}
		for _, result := range response.Results {
			if strings.Contains(result.Path, "secret") {
				t.Fatalf("glob %q leaked file outside root: %s", glob, result.Path)
			}
		}
	}
}
