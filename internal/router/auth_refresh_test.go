package router

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRefreshUserTokenSuccess(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"ghu_new","expires_in":28800,"refresh_token":"ghr_new","refresh_token_expires_in":15897600,"token_type":"bearer"}`))
	}))
	defer srv.Close()

	tok, status, err := refreshUserToken(srv.Client(), srv.URL, "cid", "csecret", "ghr_old")
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if tok.AccessToken != "ghu_new" || tok.RefreshToken != "ghr_new" || tok.ExpiresIn != 28800 {
		t.Fatalf("unexpected token: %#v", tok)
	}
	// The client secret and refresh grant must be sent to GitHub.
	if gotForm.Get("client_secret") != "csecret" || gotForm.Get("grant_type") != "refresh_token" || gotForm.Get("refresh_token") != "ghr_old" {
		t.Fatalf("unexpected form: %v", gotForm)
	}
}

func TestRefreshUserTokenGitHubErrorMapsTo401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"error":"bad_refresh_token","error_description":"expired"}`))
	}))
	defer srv.Close()

	_, status, err := refreshUserToken(srv.Client(), srv.URL, "cid", "csecret", "ghr_dead")
	if err == nil {
		t.Fatal("a rejected refresh must be an error")
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("a rejected refresh must map to 401 so the CLI prompts re-login, got %d", status)
	}
}

func TestRefreshUserTokenNoTokenIsBadGateway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token_type":"bearer"}`))
	}))
	defer srv.Close()

	if _, status, err := refreshUserToken(srv.Client(), srv.URL, "cid", "csecret", "ghr"); err == nil || status != http.StatusBadGateway {
		t.Fatalf("missing access token should be 502, got status=%d err=%v", status, err)
	}
}
