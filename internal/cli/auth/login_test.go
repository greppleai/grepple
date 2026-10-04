package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/greppleai/grepple/internal/apiclient"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func mustBackendConfig(t *testing.T) *config.Config {
	t.Helper()
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}
func TestLocalDeviceFlowStoresScopedCredentialsAndRevokesOnLogout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	var server *httptest.Server
	revoked := false
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/config":
			json.NewEncoder(w).Encode(map[string]any{"provider": "local", "clientId": "grepple-cli", "signup": true})
		case "/login/device/code":
			r.ParseForm()
			if r.FormValue("client_id") != "grepple-cli" {
				t.Errorf("client id=%q", r.FormValue("client_id"))
			}
			json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "ABCD-EFGH", "verification_uri": server.URL + "/auth/device", "expires_in": 600, "interval": 1})
		case "/login/oauth/access_token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "our-session", "token_type": "Bearer", "expires_in": 3600, "login": "person@example.test"})
		case "/auth/logout":
			if r.Header.Get("Authorization") != "Bearer our-session" {
				t.Error("logout missing scoped token")
			}
			revoked = true
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	application := cliruntime.Environment{ErrorOutput: &output}
	if err := executeLogin(application, []string{"--url", server.URL, "--no-browser"}); err != nil {
		t.Fatal(err)
	}
	credentials := mustBackendConfig(t).BackendCredentials()
	if credentials.Token != "our-session" || credentials.AuthServer != server.URL {
		t.Fatalf("credentials=%+v", credentials)
	}
	verifyPrivateBackendFile(t)
	if err := executeLogout(application, nil); err != nil {
		t.Fatal(err)
	}
	if !revoked || mustBackendConfig(t).AuthToken() != "" {
		t.Fatal("logout did not revoke and clear credentials")
	}
}
func TestLoginRejectsProviderAuthAndUntrustedVerificationOrigin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/config" {
			json.NewEncoder(w).Encode(map[string]any{"clientId": "github-client"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "ABCD-EFGH", "verification_uri": "https://attacker.example/", "expires_in": 600})
	}))
	defer server.Close()
	if err := executeLogin(cliruntime.Environment{}, []string{"--url", server.URL, "--no-browser"}); err == nil {
		t.Fatal("legacy provider login accepted")
	}
	if _, err := apiclient.New().RequestDeviceCode(t.Context(), server.URL, "grepple-cli", ""); err == nil {
		t.Fatal("untrusted verification origin accepted")
	}
	if err := executeLogin(cliruntime.Environment{}, []string{"--url", "http://remote.example", "--no-browser"}); err == nil {
		t.Fatal("insecure remote login accepted")
	}
}
func TestDevicePollingPendingAndCancellation(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		polls++
		if polls == 1 {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "opaque-session"})
	}))
	defer server.Close()
	client := apiclient.New()
	token, err := client.PollDeviceToken(t.Context(), server.URL, "grepple-cli", apiclient.DeviceCode{DeviceCode: "code", ExpiresIn: 600, Interval: 1}, func(time.Duration) {})
	if err != nil || token.AccessToken != "opaque-session" || polls != 2 {
		t.Fatalf("token=%+v err=%v polls=%d", token, err, polls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.PollDeviceToken(ctx, server.URL, "grepple-cli", apiclient.DeviceCode{ExpiresIn: 600}, nil); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func verifyPrivateBackendFile(t *testing.T) {
	t.Helper()
	path, _ := mustBackendConfig(t).BackendAuthPath()
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatalf("permissions=%v error=%v", stat, err)
	}
}
