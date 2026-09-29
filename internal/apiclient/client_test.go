package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/wire"
)

func TestFreshTokenRefreshesExpiring(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/auth/refresh" {
			t.Errorf("unexpected path %s", request.URL.Path)
		}
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"access_token":"new","expires_in":28800,"refresh_token":"new-refresh","refresh_token_expires_in":15897600}`))
	}))
	defer server.Close()
	writeExpiredToken(t, "old", "old-refresh")
	client := New(WithHTTPClient(server.Client())).(*apiClient)
	if token := client.freshToken(server.URL + "/public/search"); token != "new" {
		t.Fatalf("token=%q", token)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("refresh calls=%d", calls)
	}
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	credentials := settings.BackendCredentials()
	if credentials.Token != "new" || credentials.RefreshToken != "new-refresh" || credentials.TokenExpiry == 0 {
		t.Fatalf("refresh not persisted: %+v", credentials)
	}
}

func TestFreshTokenSkipsValidAndHonorsEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("valid token must not trigger refresh")
	}))
	defer server.Close()
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.StoreBackendLogin("valid", "refresh", 8*3600, 15897600, "user"); err != nil {
		t.Fatal(err)
	}
	client := New(WithHTTPClient(server.Client())).(*apiClient)
	if token := client.freshToken(server.URL + "/public/search"); token != "valid" {
		t.Fatalf("token=%q", token)
	}
	t.Setenv("GREPPLE_TOKEN", "environment")
	if token := client.freshToken("http://127.0.0.1:0/public/search"); token != "environment" {
		t.Fatalf("environment token did not win: %q", token)
	}
}

func TestFreshTokenFallsBackAfterRefreshFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusUnauthorized)
	}))
	defer server.Close()
	writeExpiredToken(t, "old", "old-refresh")
	client := New(WithHTTPClient(server.Client()), WithErrorOutput(ioDiscard{})).(*apiClient)
	if token := client.freshToken(server.URL + "/public/search"); token != "old" {
		t.Fatalf("token=%q", token)
	}
}

func TestFreshTokenSurvivesMalformedUserPreferences(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.StoreBackendLogin("persisted", "", 0, 0, "user"); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/settings.json"
	t.Setenv("GREPPLE_SETTINGS", path)
	if err := os.WriteFile(path, []byte(`{"anchors":`), 0o600); err != nil {
		t.Fatal(err)
	}
	client := New(WithErrorOutput(ioDiscard{})).(*apiClient)
	if token := client.freshToken("http://127.0.0.1:0/public/search"); token != "persisted" {
		t.Fatalf("backend token lost to malformed user preferences: %q", token)
	}
}
func TestValidateAnalysisResponse(t *testing.T) {
	request := wire.AnalysisRequest{Operation: wire.AnalysisGraph, Repository: "owner/repo"}
	result := analysisResult(wire.AnalysisGraph)
	valid := wire.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: true, Result: result}
	if err := validateAnalysisResponse(request, valid); err != nil {
		t.Fatal(err)
	}
	invalid := []wire.AnalysisResponse{
		{Schema: "wrong", Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: true, Result: result},
		{Schema: valid.Schema, Operation: wire.AnalysisArchitecture, Repository: "owner/repo", Found: true, Result: result},
		{Schema: valid.Schema, Operation: wire.AnalysisGraph, Repository: "other/repo", Found: true, Result: result},
		{Schema: valid.Schema, Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: false, Result: result},
		{Schema: valid.Schema, Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: true},
		{Schema: valid.Schema, Operation: wire.AnalysisGraph, Repository: "owner/repo", Found: true, Result: analysisResult(wire.AnalysisArchitecture)},
	}
	for _, response := range invalid {
		if err := validateAnalysisResponse(request, response); err == nil {
			t.Fatalf("invalid response succeeded: %+v", response)
		}
	}
}

func analysisResult(operation wire.AnalysisOperation) json.RawMessage {
	schemas := map[wire.AnalysisOperation]string{
		wire.AnalysisGraph: "grepple-navigation-graph-v7", wire.AnalysisArchitecture: "grepple-directory-architecture-v5",
		wire.AnalysisBoundaries: "grepple-boundaries-v3", wire.AnalysisResponsibilities: "grepple-directory-responsibilities-v2",
	}
	content, _ := json.Marshal(map[string]string{"schema": schemas[operation]})
	return content
}

func writeExpiredToken(t *testing.T, token, refresh string) {
	t.Helper()
	settings, err := config.LoadConfig("", true)
	if err != nil {
		t.Fatal(err)
	}
	// An imminent expiry is within the client's refresh skew.
	if err := settings.StoreBackendLogin(token, refresh, 1, 0, "user"); err != nil {
		t.Fatal(err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(content []byte) (int, error) { return len(content), nil }
