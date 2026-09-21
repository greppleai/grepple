package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/authstate"
)

func runLogin(args []string) error {
	return executeLogin(args, Dependencies{ServerDefault: func(value string) string { return value }, StoreLogin: authstate.StoreLogin, ClearToken: authstate.Clear, ConfigPath: authstate.Path})
}

// TestDeviceFlowAndTokenStorage runs the full device flow against a mock GitHub
// (device code → poll with one pending → token → user lookup) and verifies the
// token is persisted to and reloaded from the user config with 0600 perms.
func TestDeviceFlowAndTokenStorage(t *testing.T) {
	polls := 0
	srv := newDeviceFlowServer(&polls)
	defer srv.Close()
	token := runDeviceFlow(t, srv.Client(), srv.URL, &polls)
	verifyTokenPersistence(t, token)
	verifyLogoutClearsToken(t)
}

// newDeviceFlowServer mocks GitHub's device flow: one pending poll, then a
// token, and a /user lookup guarded by that token. polls counts token polls.
func newDeviceFlowServer(polls *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "DEV", "user_code": "WXYZ-1234",
				"verification_uri": "https://github.com/login/device",
				"expires_in":       900, "interval": 1,
			})
		case "/login/oauth/access_token":
			*polls++
			if *polls < 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gho_test", "token_type": "bearer", "scope": "read:user"})
		case "/user":
			if r.Header.Get("authorization") != "Bearer gho_test" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// runDeviceFlow drives requestDeviceCode -> pollDeviceToken -> login lookup
// against the mock, asserting each step, and returns the access token.
func runDeviceFlow(t *testing.T, client *http.Client, baseURL string, polls *int) string {
	t.Helper()
	dc, err := requestDeviceCode(client, baseURL, "cid", "read:user")
	if err != nil {
		t.Fatalf("requestDeviceCode: %v", err)
	}
	if dc.UserCode != "WXYZ-1234" {
		t.Fatalf("user code = %q", dc.UserCode)
	}
	tok, err := pollDeviceToken(client, baseURL, "cid", dc, func(time.Duration) {})
	if err != nil {
		t.Fatalf("pollDeviceToken: %v", err)
	}
	token := tok.AccessToken
	if token != "gho_test" {
		t.Fatalf("token = %q", token)
	}
	if *polls < 2 {
		t.Fatalf("expected polling through authorization_pending, polls=%d", *polls)
	}
	if login := fetchGitHubLogin(client, baseURL, token); login != "octocat" {
		t.Fatalf("login = %q", login)
	}
	return token
}

// verifyTokenPersistence saves the token under a fresh HOME and checks it
// reloads through both config paths with 0600 permissions.
func verifyTokenPersistence(t *testing.T, token string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	if err := authstate.SaveToken(token, "octocat"); err != nil {
		t.Fatalf("saveToken: %v", err)
	}
	if got := authstate.Token(); got != "gho_test" {
		t.Fatalf("configuredToken = %q", got)
	}
	if got := authstate.Load(); got.Token != "gho_test" || got.User != "octocat" {
		t.Fatalf("loadConfig = %+v", got)
	}
	path, _ := authstate.Path()
	if info, _ := os.Stat(path); info == nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config perms = %v", info.Mode().Perm())
	}
}

// verifyLogoutClearsToken checks that clearToken empties the stored token.
func verifyLogoutClearsToken(t *testing.T) {
	t.Helper()
	if err := authstate.Clear(); err != nil {
		t.Fatalf("clearToken: %v", err)
	}
	if got := authstate.Load().Token; got != "" {
		t.Fatalf("token after logout = %q", got)
	}
}

// TestDeviceFlowDenied verifies access_denied surfaces as an error.
func TestDeviceFlowDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "access_denied"})
	}))
	defer srv.Close()
	dc := deviceCodeResponse{DeviceCode: "D", UserCode: "U", Interval: 1, ExpiresIn: 900}
	if _, err := pollDeviceToken(srv.Client(), srv.URL, "cid", dc, func(time.Duration) {}); err == nil {
		t.Fatal("expected error for access_denied")
	}
}

// TestFetchLoginConfig verifies the client ID is read from the server, and a
// server without the endpoint yields a clear error.
func TestFetchLoginConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/config" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "Iv1.server", "scopes": "read:user repo"})
	}))
	defer srv.Close()
	cfg, err := fetchLoginConfig(srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("fetchLoginConfig: %v", err)
	}
	if cfg.ClientID != "Iv1.server" || cfg.Scopes != "read:user repo" {
		t.Fatalf("cfg = %+v", cfg)
	}

	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer bare.Close()
	if _, err := fetchLoginConfig(bare.Client(), bare.URL); err == nil {
		t.Fatal("expected error when server has no /auth/config")
	}
}

// TestRunLoginFetchesClientIDFromServer runs the full login: the client ID is
// pulled from the grepple server (--url), the device flow runs against a mock
// GitHub, and the token is persisted. No client ID is configured on the CLI.
func TestRunLoginFetchesClientIDFromServer(t *testing.T) {
	var gotClientID string
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/login/device/code":
			_ = r.ParseForm()
			gotClientID = r.FormValue("client_id")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "DEV", "user_code": "WXYZ-1234",
				"verification_uri": "https://github.com/login/device",
				"expires_in":       900, "interval": 1,
			})
		case "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gho_srv", "token_type": "bearer", "scope": "read:user"})
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "octocat"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer github.Close()
	grepple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/config" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "Iv1.fromserver", "scopes": "read:user"})
	}))
	defer grepple.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_GITHUB_HOST", github.URL)
	t.Setenv("GREPPLE_GITHUB_API", github.URL)
	if err := runLogin([]string{"--url", grepple.URL, "--no-browser"}); err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if gotClientID != "Iv1.fromserver" {
		t.Fatalf("device flow used client_id=%q, want the server-advertised one", gotClientID)
	}
	if got := authstate.Token(); got != "gho_srv" {
		t.Fatalf("configuredToken = %q", got)
	}
}

// TestRunLoginServerWithoutClientIDFails verifies a server that advertises no
// client ID produces a loud error rather than a broken device flow.
func TestRunLoginServerWithoutClientIDFails(t *testing.T) {
	grepple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"clientId": "", "scopes": "read:user"})
	}))
	defer grepple.Close()
	t.Setenv("HOME", t.TempDir())
	if err := runLogin([]string{"--url", grepple.URL, "--no-browser"}); err == nil {
		t.Fatal("expected error when server advertises no client ID")
	}
}
