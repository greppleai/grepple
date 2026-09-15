package aiprovider

import (
	"os"
	"path/filepath"
	"testing"
)

type testCredentials struct {
	Token string `json:"token"`
}

func TestStoreSeparatesProvidersAndUsesPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv(CredentialsPathEnv, path)
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save("one", testCredentials{Token: "secret-one"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("two", testCredentials{Token: "secret-two"}); err != nil {
		t.Fatal(err)
	}
	var loaded testCredentials
	found, err := store.Load("one", &loaded)
	if err != nil || !found || loaded.Token != "secret-one" {
		t.Fatalf("loaded=%+v found=%v err=%v", loaded, found, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential mode = %o", info.Mode().Perm())
	}
	if err := store.Delete("one"); err != nil {
		t.Fatal(err)
	}
	found, err = store.Load("one", &loaded)
	if err != nil || found {
		t.Fatalf("deleted provider found=%v err=%v", found, err)
	}
	found, err = store.Load("two", &loaded)
	if err != nil || !found || loaded.Token != "secret-two" {
		t.Fatalf("other provider was changed: found=%v loaded=%+v err=%v", found, loaded, err)
	}
}
