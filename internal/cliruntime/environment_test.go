package cliruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/search"
)

func TestLoadRepositoryConfigFindsAncestorAndKeepsAuthenticationUserOwned(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "server": "https://repo.example",
  "ignore": {"paths": ["sandbox/**"]},
  "output": {"spillThresholdBytes": 4096}
}
`
	if err := writeConfigFixture(root, content); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, child)

	config, path, err := LoadInvocationRepositoryConfig(RepositoryInvocationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, ".grepple", "grepple.json") || config.Server != "https://repo.example" || len(config.Ignore.Paths) != 1 || config.Output.SpillThresholdBytes != 4096 {
		t.Fatalf("repository config path=%q config=%+v", path, config)
	}
}

func TestRepositoryConfigLoadsIndexPatterns(t *testing.T) {
	root := t.TempDir()
	content := `{"index":{"repositories":[{"repo":"sourcegraph/zoekt","branches":["main","release/*"],"tags":["v0.25.*"]}]}}`
	if err := writeConfigFixture(root, content); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	config, _, err := LoadInvocationRepositoryConfig(RepositoryInvocationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Index.Repositories) != 1 || config.Index.Repositories[0].Repo != "sourcegraph/zoekt" {
		t.Fatalf("unexpected index config: %+v", config.Index)
	}
}

func TestRepositoryConfigRejectsAuthenticationFields(t *testing.T) {
	root := t.TempDir()
	if err := writeConfigFixture(root, `{"token":"repository-secret"}`); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	if _, _, err := LoadInvocationRepositoryConfig(RepositoryInvocationOptions{}); err == nil {
		t.Fatal("expected repository authentication field error")
	}
}
func TestRepositoryConfigRejectsInvalidSourceAndOutputSettings(t *testing.T) {
	for _, content := range []string{
		`{"ignore":{"paths":["../outside/**"]}}`,
		`{"output":{"spillThresholdBytes":-1}}`,
		`{"index":{"repositories":[{"repo":"invalid","tags":["v*"]}]}}`,
		`{"index":{"repositories":[{"repo":"owner/repo","branches":["["]}]}}`,
		`{"unknown":true}`,
		`{} {}`,
	} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			if err := writeConfigFixture(root, content); err != nil {
				t.Fatal(err)
			}
			chdirForConfigTest(t, root)
			if _, _, err := LoadInvocationRepositoryConfig(RepositoryInvocationOptions{}); err == nil {
				t.Fatalf("expected invalid repository config for %s", content)
			}
		})
	}
}

func TestLoadConfigDoesNotOverlayUserTokenFromRepository(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".grepple"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grepple", "config.json"), []byte(`{"token":"user-token","server":"https://user.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFixture(root, `{"server":"https://repo.example"}`); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	settings, err := config.LoadConfig(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AuthToken() != "user-token" || settings.ServerDefault("") != "https://repo.example" {
		t.Fatalf("merged config = %+v", settings)
	}
	bypassed, err := config.LoadConfig(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if bypassed.AuthToken() != "user-token" || bypassed.ServerDefault("") != "https://user.example" {
		t.Fatalf("bypassed repository config = %+v", bypassed)
	}
}
func TestRepositoryIgnoreAppliesToRecursiveDiscoveryAndExplicitFileBypasses(t *testing.T) {
	root := t.TempDir()
	ignoredDirectory := filepath.Join(root, "sandbox")
	if err := os.MkdirAll(ignoredDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	ignoredFile := filepath.Join(ignoredDirectory, "ignored.go")
	if err := os.WriteFile(ignoredFile, []byte("package sandbox\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigFixture(root, `{"ignore":{"paths":["sandbox/**"]}}`); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)

	params := search.Params{Files: true, Globs: []string{"."}}
	if err := search.ConfigureSourcePolicy(&params, NewRepository(RepositoryInvocationOptions{}, os.Stderr)); err != nil {
		t.Fatal(err)
	}
	paths, err := search.ListFilePaths(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "main.go" {
		t.Fatalf("recursive paths = %#v", paths)
	}

	params.Globs = []string{ignoredFile}
	paths, err = search.ListFilePaths(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "sandbox/ignored.go" {
		t.Fatalf("explicit paths = %#v", paths)
	}
}
func TestRepositoryConfigIgnoresLegacyRootFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"server":"https://legacy.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	config, path, err := LoadInvocationRepositoryConfig(RepositoryInvocationOptions{})
	if err != nil || path != "" || config.Server != "" {
		t.Fatalf("legacy config loaded: %+v %q %v", config, path, err)
	}
	if err := writeConfigFixture(root, `{"server":"https://current.example"}`); err != nil {
		t.Fatal(err)
	}
	config, path, err = LoadInvocationRepositoryConfig(RepositoryInvocationOptions{})
	if err != nil || path != filepath.Join(root, ".grepple", "grepple.json") || config.Server != "https://current.example" {
		t.Fatalf("relocated config = %+v %q %v", config, path, err)
	}
}

func writeConfigFixture(root, content string) error {
	if err := os.MkdirAll(filepath.Join(root, ".grepple"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".grepple", "grepple.json"), []byte(content), 0o600)
}

func chdirForConfigTest(t *testing.T, directory string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}
