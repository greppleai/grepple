package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

// githubAuthMock serves /user and /user/memberships/orgs/{org} keyed by token:
//
//	member-token   -> valid user, active member of testorg
//	outsider-token -> valid user, not a member (404)
//	bad-token      -> 401
func githubAuthMock(t *testing.T, userHits *int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("authorization"), "Bearer ")
		w.Header().Set("content-type", "application/json")
		switch {
		case r.URL.Path == "/user":
			if userHits != nil {
				atomic.AddInt64(userHits, 1)
			}
			if token == "bad-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"login": "user-" + token})
		case r.URL.Path == "/user/memberships/orgs/testorg":
			if token == "member-token" {
				_ = json.NewEncoder(w).Encode(map[string]any{"state": "active"})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func newAuthState(apiBase string, required bool) *routerState {
	backends := []string{"http://127.0.0.1:9"}
	options := routerOptions{
		backends:      backends,
		timeout:       2 * time.Second,
		concurrency:   1,
		githubAPIBase: apiBase,
		authRequired:  required,
		authOrgs:      []string{"testorg"},
		ring:          newRing(backends),
	}
	return &routerState{o: options, q: newQueue(options), authCache: newTokenAuthCache()}
}

// status hits /public/raw without a repo: the proxy returns 400 once the
// middleware lets the request through, so 400 means "authorized", while
// 401/403 mean the middleware rejected it.
func status(t *testing.T, app *fiber.App, token string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/public/raw", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestPublicEndpointsRequireOrgMembership(t *testing.T) {
	api := githubAuthMock(t, nil)
	app := newAuthState(api, true).handler()

	if got := status(t, app, ""); got != http.StatusUnauthorized {
		t.Errorf("missing token: status=%d want 401", got)
	}
	if got := status(t, app, "bad-token"); got != http.StatusUnauthorized {
		t.Errorf("invalid token: status=%d want 401", got)
	}
	if got := status(t, app, "outsider-token"); got != http.StatusForbidden {
		t.Errorf("non-member: status=%d want 403", got)
	}
	if got := status(t, app, "member-token"); got != http.StatusBadRequest {
		t.Errorf("member: status=%d want 400 (passed auth, proxy wants repo)", got)
	}
}

func TestAuthDisabledIsPassThrough(t *testing.T) {
	api := githubAuthMock(t, nil)
	app := newAuthState(api, false).handler()
	// No token, auth not required -> reaches proxy -> 400 (needs repo), not 401.
	if got := status(t, app, ""); got != http.StatusBadRequest {
		t.Errorf("auth disabled: status=%d want 400 pass-through", got)
	}
}

func TestValidatedTokenIsCached(t *testing.T) {
	var userHits int64
	api := githubAuthMock(t, &userHits)
	app := newAuthState(api, true).handler()

	if got := status(t, app, "member-token"); got != http.StatusBadRequest {
		t.Fatalf("first call status=%d", got)
	}
	if got := status(t, app, "member-token"); got != http.StatusBadRequest {
		t.Fatalf("second call status=%d", got)
	}
	if hits := atomic.LoadInt64(&userHits); hits != 1 {
		t.Errorf("GitHub /user hits=%d, want 1 (second request served from cache)", hits)
	}
}
