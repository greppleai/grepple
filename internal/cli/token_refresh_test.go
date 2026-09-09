package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func writeConfigFile(t *testing.T, path string, c config) {
	t.Helper()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestFreshTokenRefreshesExpiring verifies an expiring stored token is renewed
// via the server's /auth/refresh shortly before it expires, and the new token
// set is persisted.
func TestFreshTokenRefreshesExpiring(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "") // don't let an ambient env token win

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/refresh" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		atomic.AddInt32(&calls, 1)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ghu_new","expires_in":28800,"refresh_token":"ghr_new","refresh_token_expires_in":15897600}`))
	}))
	defer srv.Close()

	// Stored token already expired 1 minute ago.
	if err := storeLogin("ghu_old", "ghr_old", 0, 0, "octocat"); err != nil {
		t.Fatal(err)
	}
	// storeLogin with expiresIn=0 records no expiry; force a past expiry directly.
	writeExpiredToken(t, "ghu_old", "ghr_old")

	got := freshToken(srv.URL + "/public/search")
	if got != "ghu_new" {
		t.Fatalf("expected refreshed token, got %q", got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected exactly one refresh call, got %d", calls)
	}
	// Persisted?
	c := loadConfig()
	if c.Token != "ghu_new" || c.RefreshToken != "ghr_new" || c.TokenExpiry == 0 {
		t.Fatalf("refresh not persisted: %#v", c)
	}
	if c.User != "octocat" {
		t.Fatalf("refresh must preserve the stored user, got %q", c.User)
	}
}

// TestFreshTokenSkipsWhenValid verifies a comfortably valid token is returned
// without contacting the server.
func TestFreshTokenSkipsWhenValid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("valid token must not trigger a refresh call")
	}))
	defer srv.Close()

	// Expires in 8 hours — well beyond the skew window.
	if err := storeLogin("ghu_valid", "ghr", 8*3600, 15897600, "octocat"); err != nil {
		t.Fatal(err)
	}
	if got := freshToken(srv.URL + "/public/search"); got != "ghu_valid" {
		t.Fatalf("got %q", got)
	}
}

// TestFreshTokenEnvWins verifies GREPPLE_TOKEN is used verbatim and never refreshed.
func TestFreshTokenEnvWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "env_token")
	_ = storeLogin("ghu_old", "ghr_old", 0, 0, "octocat")
	writeExpiredToken(t, "ghu_old", "ghr_old")
	if got := freshToken("http://127.0.0.1:0/public/search"); got != "env_token" {
		t.Fatalf("env token must win, got %q", got)
	}
}

// TestFreshTokenDegradesOnRefreshFailure verifies a refresh error is non-fatal:
// the stale token is returned so the request can still surface a real 401.
func TestFreshTokenDegradesOnRefreshFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_ = storeLogin("ghu_old", "ghr_old", 0, 0, "octocat")
	writeExpiredToken(t, "ghu_old", "ghr_old")
	if got := freshToken(srv.URL + "/public/search"); got != "ghu_old" {
		t.Fatalf("expected stale token fallback, got %q", got)
	}
}

// writeExpiredToken rewrites the stored config so the access token expired a
// minute ago (storeLogin can only set future/zero expiries).
func writeExpiredToken(t *testing.T, token, refresh string) {
	t.Helper()
	path, err := userConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	c := loadConfig()
	c.Token = token
	c.RefreshToken = refresh
	c.TokenExpiry = time.Now().Add(-time.Minute).Unix()
	c.RefreshExpiry = time.Now().Add(30 * 24 * time.Hour).Unix()
	writeConfigFile(t, path, c)
}
