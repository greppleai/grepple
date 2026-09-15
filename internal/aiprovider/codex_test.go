package aiprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
)

func TestCodexDeviceLoginPersistsAccountCredentials(t *testing.T) {
	access := testJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())})
	idToken := testJWT(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "account-123"}})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("content-type", "application/json")
		switch request.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			fmt.Fprint(writer, `{"device_auth_id":"device","user_code":"CODE","interval":"1"}`)
		case "/api/accounts/deviceauth/token":
			fmt.Fprint(writer, `{"authorization_code":"authorization","code_verifier":"verifier"}`)
		case "/oauth/token":
			if err := request.ParseForm(); err != nil || request.Form.Get("grant_type") != "authorization_code" {
				t.Fatalf("unexpected token form: %v %#v", err, request.Form)
			}
			fmt.Fprintf(writer, `{"access_token":%q,"refresh_token":"refresh","id_token":%q}`, access, idToken)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store := testStore(t)
	provider := newCodexProvider(store, server.Client())
	provider.issuer = server.URL
	var output strings.Builder
	if err := provider.Login(context.Background(), LoginOptions{NoBrowser: true, Output: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "CODE") {
		t.Fatalf("login prompt = %q", output.String())
	}
	var credentials codexCredentials
	found, err := store.Load(provider.Name(), &credentials)
	if err != nil || !found || credentials.AccountID != "account-123" || credentials.RefreshToken != "refresh" {
		t.Fatalf("credentials=%+v found=%v err=%v", credentials, found, err)
	}
}

func TestCodexRefreshRotatesTokens(t *testing.T) {
	newAccess := testJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/oauth/token" {
			http.NotFound(writer, request)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["grant_type"] != "refresh_token" {
			t.Fatalf("refresh body=%#v err=%v", body, err)
		}
		fmt.Fprintf(writer, `{"access_token":%q,"refresh_token":"rotated"}`, newAccess)
	}))
	defer server.Close()

	store := testStore(t)
	expired := testJWT(map[string]any{"exp": float64(time.Now().Add(-time.Hour).Unix())})
	if err := store.Save(codexProviderName, codexCredentials{AccessToken: expired, RefreshToken: "old", AccountID: "account"}); err != nil {
		t.Fatal(err)
	}
	provider := newCodexProvider(store, server.Client())
	provider.issuer = server.URL
	credentials, err := provider.credentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessToken != newAccess || credentials.RefreshToken != "rotated" {
		t.Fatalf("credentials were not refreshed: %+v", credentials)
	}
}

func TestCodexLanguageModelUsesOAuthHeaders(t *testing.T) {
	var gotAccount, gotAuthorization, gotOriginator, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAccount = request.Header.Get("ChatGPT-Account-ID")
		gotAuthorization = request.Header.Get("Authorization")
		gotOriginator = request.Header.Get("originator")
		gotPath = request.URL.Path
		writer.Header().Set("content-type", "application/json")
		fmt.Fprint(writer, `{"id":"response","object":"response","created_at":1,"status":"completed","model":"gpt-5.3-codex","output":[{"id":"message","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	store := testStore(t)
	access := testJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())})
	if err := store.Save(codexProviderName, codexCredentials{AccessToken: access, RefreshToken: "refresh", AccountID: "account-123"}); err != nil {
		t.Fatal(err)
	}
	provider := newCodexProvider(store, server.Client())
	provider.apiURL = server.URL
	model, err := provider.LanguageModel(context.Background(), "gpt-5.3-codex")
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.Generate(context.Background(), fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("question")}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content.Text() != "answer" || gotPath != "/responses" || gotAccount != "account-123" || gotAuthorization != "Bearer "+access || gotOriginator != "grepple" {
		t.Fatalf("response=%q path=%q account=%q authorization=%q originator=%q", response.Content.Text(), gotPath, gotAccount, gotAuthorization, gotOriginator)
	}
}

func TestRegistryRejectsUnknownProvider(t *testing.T) {
	registry := NewRegistry(testStore(t), http.DefaultClient)
	if _, err := registry.Provider("unknown"); err == nil {
		t.Fatal("unknown provider succeeded")
	}
	want := []string{"anthropic", "anthropic-subscription", "bedrock", "codex", "copilot", "openai"}
	if got := registry.Names(); !slices.Equal(got, want) {
		t.Fatalf("providers = %v, want %v", got, want)
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv(CredentialsPathEnv, t.TempDir()+"/credentials.json")
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testJWT(payload map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	content, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(content) + ".signature"
}
