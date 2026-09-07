package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestFiberHTTPRoutesAndProxy(t *testing.T) {
	backend := newProxyTestBackend()
	defer backend.Close()
	app := newTestRouterApp(backend.URL + "/base")
	verifyCoreRoutes(t, app)
	verifyProxyBehavior(t, app)
	verifyProxyRedirect(t, app)
}

// newProxyTestBackend serves a minimal shard: health/index plus a /base/raw
// that echoes its path query and exercises header forwarding, multi-value
// cookies, hop-by-hop stripping, and redirects.
func newProxyTestBackend() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/base/health":
			response.Header().Set("content-type", "application/json")
			_, _ = response.Write([]byte(`{"ok":true}`))
		case "/base/index":
			response.Header().Set("content-type", "application/json")
			_, _ = response.Write([]byte(`{"repos":[]}`))
		case "/base/raw":
			if request.URL.Query().Get("path") == "redirect" {
				http.Redirect(response, request, "/target", http.StatusFound)
				return
			}
			response.Header().Set("x-backend", "yes")
			response.Header().Set("x-seen-request", request.Header.Get("x-test-request"))
			response.Header().Add("set-cookie", "first=1")
			response.Header().Add("set-cookie", "second=2")
			response.Header().Set("connection", "x-hop")
			response.Header().Set("x-hop", "remove-me")
			_, _ = response.Write([]byte(request.URL.Query().Get("path")))
		default:
			http.NotFound(response, request)
		}
	}))
}

// newTestRouterApp builds the router app wired to a single backend URL.
func newTestRouterApp(backendURL string) *fiber.App {
	options := routerOptions{
		backends:    []string{backendURL},
		timeout:     time.Second,
		concurrency: 1,
		ring:        newRing([]string{backendURL}),
	}
	state := &routerState{o: options}
	state.q = newQueue(options)
	return state.handler()
}

// verifyCoreRoutes checks the liveness/readiness/jobs/overview routes answer
// with their expected status and JSON, and that a malformed search body and an
// unknown path fail as specified.
func verifyCoreRoutes(t *testing.T, app *fiber.App) {
	t.Helper()
	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/livez", "", http.StatusOK},
		{http.MethodGet, "/health", "", http.StatusOK},
		{http.MethodGet, "/", "", http.StatusOK},
		{http.MethodGet, "/jobs", "", http.StatusOK},
		{http.MethodPost, "/public/search", "not-json", http.StatusBadRequest},
		{http.MethodPost, "/github/webhook", "{}", http.StatusOK},
		{http.MethodGet, "/missing", "", http.StatusNotFound},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response, err := app.Test(request)
		if err != nil {
			t.Fatalf("%s %s: %v", test.method, test.path, err)
		}
		if response.StatusCode != test.status {
			t.Errorf("%s %s status=%d want %d", test.method, test.path, response.StatusCode, test.status)
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Errorf("%s %s returned invalid JSON: %v", test.method, test.path, err)
		}
		response.Body.Close()
	}
}

// verifyProxyBehavior checks /public/raw proxies to the owning shard: the
// request header is forwarded, both set-cookie values survive, hop-by-hop
// headers are stripped, and the body passes through.
func verifyProxyBehavior(t *testing.T, app *fiber.App) {
	t.Helper()
	proxyRequest := httptest.NewRequest(http.MethodGet, "/public/raw?repo=owner/repo&path=README.md", nil)
	proxyRequest.Header.Set("x-test-request", "forwarded")
	response, err := app.Test(proxyRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("x-backend") != "yes" {
		t.Fatalf("proxy status=%d headers=%v", response.StatusCode, response.Header)
	}
	if response.Header.Get("x-seen-request") != "forwarded" {
		t.Fatalf("request header was not forwarded: %v", response.Header)
	}
	if cookies := response.Header.Values("set-cookie"); len(cookies) != 2 {
		t.Fatalf("set-cookie headers=%v", cookies)
	}
	if response.Header.Get("x-hop") != "" {
		t.Fatalf("hop-by-hop header was forwarded: %v", response.Header)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != "README.md" {
		t.Fatalf("proxy body=%q", body)
	}
	response.Body.Close()
}

// verifyProxyRedirect checks a backend redirect is relayed (status and
// location preserved) rather than followed.
func verifyProxyRedirect(t *testing.T, app *fiber.App) {
	t.Helper()
	redirect, err := app.Test(httptest.NewRequest(http.MethodGet, "/public/raw?repo=owner/repo&path=redirect", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer redirect.Body.Close()
	if redirect.StatusCode != http.StatusFound || redirect.Header.Get("location") != "/target" {
		t.Fatalf("redirect status=%d location=%q", redirect.StatusCode, redirect.Header.Get("location"))
	}
}

// TestPublicReposAggregatesShards verifies GET /public/repos returns the union
// of repos reported by each shard's /index, sorted by OWNER/REPO name.
func TestPublicReposAggregatesShards(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"repos":[{"repo":"acme/web"},{"repo":"acme/api"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer backend.Close()

	options := routerOptions{
		backends:    []string{backend.URL},
		timeout:     time.Second,
		concurrency: 1,
		ring:        newRing([]string{backend.URL}),
	}
	state := &routerState{o: options}
	state.q = newQueue(options)
	app := state.handler()

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/public/repos", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Count int  `json:"count"`
		Repos []struct {
			Repo  string `json:"repo"`
			Shard string `json:"shard"`
		} `json:"repos"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 2 || len(payload.Repos) != 2 {
		t.Fatalf("count=%d repos=%d", payload.Count, len(payload.Repos))
	}
	if payload.Repos[0].Repo != "acme/api" || payload.Repos[1].Repo != "acme/web" {
		t.Fatalf("repos not sorted: %+v", payload.Repos)
	}
	if payload.Repos[0].Shard != backend.URL {
		t.Fatalf("shard not populated: %q", payload.Repos[0].Shard)
	}
}

// TestJobsOnlyOnJobsEndpoint asserts job stats are exposed solely by /jobs and
// no longer leak into /, /health, or the liveness endpoint.
func TestJobsOnlyOnJobsEndpoint(t *testing.T) {
	options := routerOptions{backends: []string{"http://127.0.0.1:1"}, timeout: 50 * time.Millisecond, ring: newRing([]string{"http://127.0.0.1:1"})}
	state := &routerState{o: options, q: newQueue(options)}
	app := state.handler()

	get := func(path string) map[string]any {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer resp.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("%s: invalid JSON: %v", path, err)
		}
		return payload
	}

	if _, ok := get("/")["jobs"]; ok {
		t.Error("/ must not include jobs")
	}
	if _, ok := get("/health")["jobs"]; ok {
		t.Error("/health must not include jobs")
	}
	livez := get("/livez")
	if livez["ok"] != true {
		t.Errorf("/livez ok=%v want true", livez["ok"])
	}
	if _, ok := livez["jobs"]; ok {
		t.Error("/livez must not include jobs")
	}
	if _, ok := get("/jobs")["pending"]; !ok {
		t.Error("/jobs must expose job stats (pending)")
	}
}

// TestAuthConfigAdvertisesClientID verifies /auth/config serves the router's
// configured GitHub client ID and scopes to CLI login clients.
func TestAuthConfigAdvertisesClientID(t *testing.T) {
	backends := []string{"http://127.0.0.1:9"}
	options := routerOptions{
		backends:       backends,
		timeout:        time.Second,
		concurrency:    1,
		githubClientID: "Iv1.router",
		githubScopes:   "read:user",
		ring:           newRing(backends),
	}
	state := &routerState{o: options, q: newQueue(options)}
	app := state.handler()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/auth/config", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["clientId"] != "Iv1.router" || payload["scopes"] != "read:user" {
		t.Fatalf("payload=%v", payload)
	}
}
