package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWebhookDispatch verifies the router reacts to each GitHub event it
// supports: push (re-index), branch delete (ignore), and repository
// created/deleted/archived/unarchived.
func TestWebhookDispatch(t *testing.T) {
	shard := newFakeShard(true)
	backend := httptest.NewServer(shard.handler())
	defer backend.Close()
	backends := []string{backend.URL}
	options := routerOptions{backends: backends, timeout: time.Second, concurrency: 1, ring: newRing(backends)}
	state := &routerState{o: options, q: newQueue(options)}
	app := state.handler()

	repo := `"repository":{"full_name":"owner/repo","clone_url":"https://x/repo.git","default_branch":"trunk"}`
	cases := []struct {
		name, event, body, wantAction string
	}{
		{"push", "push", `{"ref":"refs/heads/main",` + repo + `}`, "pull"},
		{"branch-delete", "push", `{"ref":"refs/heads/x","deleted":true,` + repo + `}`, "ignored (branch deleted)"},
		{"created", "repository", `{"action":"created",` + repo + `}`, "clone"},
		{"deleted", "repository", `{"action":"deleted",` + repo + `}`, "remove"},
		{"archived", "repository", `{"action":"archived",` + repo + `}`, "remove (archived)"},
		{"unarchived", "repository", `{"action":"unarchived",` + repo + `}`, "clone (unarchived)"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/github/webhook", strings.NewReader(c.body))
		req.Header.Set("x-github-event", c.event)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var m map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&m)
		resp.Body.Close()
		if m["action"] != c.wantAction {
			t.Errorf("%s: action=%v want %q", c.name, m["action"], c.wantAction)
		}
	}
}
