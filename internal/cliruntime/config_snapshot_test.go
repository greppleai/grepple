package cliruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/config"
)

func TestNewContextSharesInvocationConfigSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".grepple", "grepple.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"server":"https://first.example","ignore":{"paths":["first/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	askPath := filepath.Join(home, ".grepple", "grepple.json")
	if err := os.MkdirAll(filepath.Dir(askPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(askPath, []byte(`{"ask":{"model":"first-model"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_SERVER", "")
	settings, err := config.LoadConfig(root, false)
	if err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	context := NewContext(ContextOptions{Config: settings})
	if err := os.WriteFile(path, []byte(`{"server":"https://second.example","ignore":{"paths":["second/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(askPath, []byte(`{"ask":{"model":"second-model"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := context.Configuration().ServerDefault(""); got != "https://first.example" {
		t.Fatalf("server reloaded: %q", got)
	}
	if ask, err := context.Configuration().AskPreferences(); err != nil || ask.Model != "first-model" {
		t.Fatalf("Ask settings reloaded: %+v, %v", ask, err)
	}
	assertRepositorySnapshot(t, context, path)
}

func assertRepositorySnapshot(t *testing.T, context Context, path string) {
	t.Helper()
	for range 2 {
		scope, err := context.Repository().ScopeOptions()
		if err != nil || len(scope.IgnorePaths) != 1 || scope.IgnorePaths[0] != "first/**" {
			t.Fatalf("scope reloaded: %+v, %v", scope, err)
		}
		if got, err := context.Repository().ConfigurationPath(); got != path || err != nil {
			t.Fatalf("config path = %q, %v", got, err)
		}
	}
}
