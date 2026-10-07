package agentskills

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteSourcePinsTagWithoutCookiesOrRedirects(t *testing.T) {
	original, err := NewRemoteSource("0.0.5")
	if err != nil {
		t.Fatal(err)
	}
	source := original.(*remoteSource)
	if source.base != "https://raw.githubusercontent.com/greppleai/grepple/v0.0.5/" {
		t.Fatal(source.base)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("implicit credentials")
		}
		http.Redirect(w, r, "/other", http.StatusFound)
	}))
	defer server.Close()
	source.base = server.URL + "/"
	_, err = source.Read(context.Background(), catalogPath)
	if err == nil || calls != 1 {
		t.Fatal("followed redirect or accepted non-200 source")
	}
}
func TestRemoteSourceBoundsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, strings.Repeat("x", maxFileBytes+1)) }))
	defer server.Close()
	original, _ := NewRemoteSource("v0.0.5")
	source := original.(*remoteSource)
	source.base = server.URL + "/"
	if _, err := source.Read(context.Background(), "SKILL.md"); err == nil {
		t.Fatal("oversized source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Read(ctx, "SKILL.md"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestLocalSourceRefusesEscapesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	source, err := NewLocalSource(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside", "/absolute", "a\\b"} {
		if _, err := source.Read(context.Background(), name); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err = os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(root, "SKILL.md")); err != nil {
		t.Skip(err)
	}
	if _, err = source.Read(context.Background(), "SKILL.md"); err == nil {
		t.Fatal("followed source symlink")
	}
}
