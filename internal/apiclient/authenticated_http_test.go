package apiclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/greppleai/grepple/internal/config"
)

func TestAuthenticatedRedirectCannotCrossServerPort(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved-session", true: "explicit-override"}[override], func(t *testing.T) {
			verifyAuthenticatedPortRedirect(t, override)
		})
	}
}

func verifyAuthenticatedPortRedirect(t *testing.T, override bool) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		w.Write([]byte(`{"ok":true,"repos":[]}`))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Error("initial request missing credential")
		}
		http.Redirect(w, r, target.URL+"/public/repos", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	if override {
		t.Setenv("GREPPLE_TOKEN", "session")
	} else {
		storeTestSession(t, "session", origin.URL)
	}
	if _, err := New().Repos(t.Context(), origin.URL); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if forwarded.Load() != 0 {
		t.Fatal("request reached another origin")
	}
}

func storeTestSession(t *testing.T, token, server string) {
	t.Helper()
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.StoreBackendLogin(token, "", 0, 0, "user", server); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatedSameOriginRedirectRetainsCustomPolicy(t *testing.T) {
	t.Setenv("GREPPLE_TOKEN", "session")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session" {
			t.Error("missing credential")
		}
		if r.URL.Path == "/public/repos" {
			http.Redirect(w, r, "/canonical", http.StatusTemporaryRedirect)
			return
		}
		w.Write([]byte(`{"ok":true,"repos":[{"repo":"example/repo"}]}`))
	}))
	defer server.Close()
	var redirects atomic.Int32
	transport := server.Client()
	transport.CheckRedirect = func(*http.Request, []*http.Request) error {
		redirects.Add(1)
		return nil
	}
	repos, err := New(WithHTTPClient(transport)).Repos(t.Context(), server.URL)
	if err != nil || len(repos) != 1 || redirects.Load() != 1 {
		t.Fatalf("repos=%v err=%v redirects=%d", repos, err, redirects.Load())
	}
}

func TestRevokeLoginUsesSavedSessionDespiteEnvironmentOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "different-session")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/logout" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer saved-session" {
			t.Error("logout did not revoke the saved session")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.StoreBackendLogin("saved-session", "", 0, 0, "user", server.URL); err != nil {
		t.Fatal(err)
	}
	if err := New().RevokeLogin(t.Context(), server.URL); err != nil {
		t.Fatal(err)
	}
	if err := New().RevokeLogin(t.Context(), "https://other.example"); err == nil {
		t.Fatal("logout accepted another server")
	}
}

func TestRevokeLoginRejectsRedirectAndFailedStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	var forwarded atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status == http.StatusTemporaryRedirect {
				http.Redirect(w, r, target.URL+"/auth/logout", status)
				return
			}
			w.WriteHeader(status)
		}))
		settings, err := config.LoadConfig("", true)
		if err != nil {
			t.Fatal(err)
		}
		if err := settings.StoreBackendLogin("saved-session", "", 0, 0, "user", server.URL); err != nil {
			t.Fatal(err)
		}
		if err := New().RevokeLogin(t.Context(), server.URL); err == nil {
			t.Fatalf("logout accepted HTTP %d", status)
		}
		server.Close()
	}
	if forwarded.Load() != 0 {
		t.Fatal("logout followed a redirect")
	}
}
