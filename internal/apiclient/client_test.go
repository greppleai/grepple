package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/authstate"
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
	config := authstate.Load()
	if config.Token != "new" || config.RefreshToken != "new-refresh" || config.TokenExpiry == 0 {
		t.Fatalf("refresh not persisted: %#v", config)
	}
}

func TestFreshTokenSkipsValidAndHonorsEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("valid token must not trigger refresh")
	}))
	defer server.Close()
	if err := authstate.StoreLogin("valid", "refresh", 8*3600, 15897600, "user"); err != nil {
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
	if err := authstate.StoreLogin(token, refresh, 0, 0, "user"); err != nil {
		t.Fatal(err)
	}
	path, err := authstate.Path()
	if err != nil {
		t.Fatal(err)
	}
	config := authstate.Load()
	config.TokenExpiry = time.Now().Add(-time.Minute).Unix()
	content, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(content []byte) (int, error) { return len(content), nil }
