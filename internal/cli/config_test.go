package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/search"
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
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, child)

	config, path, err := loadRepositoryConfig()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "grepple.json") || config.Server != "https://repo.example" || len(config.Ignore.Paths) != 1 || config.Output.SpillThresholdBytes != 4096 {
		t.Fatalf("repository config path=%q config=%+v", path, config)
	}
}

func TestRepositoryConfigRejectsAuthenticationFields(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"token":"repository-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	if _, _, err := loadRepositoryConfig(); err == nil {
		t.Fatal("expected repository authentication field error")
	}
}
func TestRepositoryConfigRejectsInvalidSourceAndOutputSettings(t *testing.T) {
	for _, content := range []string{
		`{"ignore":{"paths":["../outside/**"]}}`,
		`{"output":{"spillThresholdBytes":-1}}`,
		`{"unknown":true}`,
		`{} {}`,
	} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			chdirForConfigTest(t, root)
			if _, _, err := loadRepositoryConfig(); err == nil {
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
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"server":"https://repo.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	config := loadConfig()
	if config.Token != "user-token" || config.Server != "https://repo.example" {
		t.Fatalf("merged config = %+v", config)
	}
	previous := activeRepositoryOptions
	activeRepositoryOptions.disabled = true
	bypassed := loadConfig()
	activeRepositoryOptions = previous
	if bypassed.Token != "user-token" || bypassed.Server != "https://user.example" {
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
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"ignore":{"paths":["sandbox/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)

	params := search.Params{Files: true, Globs: []string{"."}}
	if err := applyRepositorySourceConfig(&params); err != nil {
		t.Fatal(err)
	}
	paths, err := search.ListFilePaths(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "grepple.json" || paths[1] != "main.go" {
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
