package cli

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// isolateCLIAuth points HOME at a temp dir and clears GREPPLE_TOKEN/GREPPLE_SERVER so
// a remote CLI test never reads the developer's real login/config or triggers a
// token refresh.
func isolateCLIAuth(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	t.Setenv("GREPPLE_SERVER", "")
}

func TestRulesAddPostsRule(t *testing.T) {
	isolateCLIAuth(t)
	var (
		gotPath   string
		gotMethod string
		gotRule   api.Rule
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotRule)
		gotRule.ID = "uses-checkout"
		gotRule.Mode = api.RuleModeCount
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(gotRule)
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runRules([]string{"add", "--server", server.URL, "--name", "Uses Checkout",
			"--repo", "owner/*", "actions/checkout", "**/*.yml"}); err != nil {
			t.Fatal(err)
		}
	})

	if gotMethod != http.MethodPost || gotPath != "/public/rules" {
		t.Fatalf("expected POST /public/rules, got %s %s", gotMethod, gotPath)
	}
	if gotRule.Request.Query == nil || *gotRule.Request.Query != "actions/checkout" {
		t.Fatalf("query not sent: %#v", gotRule.Request)
	}
	if len(gotRule.Request.Globs) != 1 || gotRule.Request.Globs[0] != "**/*.yml" {
		t.Fatalf("globs not sent: %#v", gotRule.Request.Globs)
	}
	if !strings.Contains(out, "created rule uses-checkout") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRulesResultsRenders(t *testing.T) {
	isolateCLIAuth(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/results") {
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(api.RuleResults{Rule: "r1", Mode: api.RuleModeCount, Repos: []api.RuleRepoResult{
			{Repo: "owner/alpha", Files: 2, Matches: 3},
			{Repo: "owner/beta", Files: 1, Matches: 1},
		}})
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runRules([]string{"results", "--server", server.URL, "r1"}); err != nil {
			t.Fatal(err)
		}
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "owner/alpha\t2 files\t3 matches") {
		t.Fatalf("unexpected results output: %q", out)
	}
}

func TestRulesListRenders(t *testing.T) {
	isolateCLIAuth(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.RuleSet{Generation: 2, Rules: []api.Rule{
			{ID: "a", Mode: api.RuleModeCount, Name: "Alpha"},
		}})
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runRules([]string{"list", "--server", server.URL}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "a\tcount\tAlpha") {
		t.Fatalf("unexpected list output: %q", out)
	}
}
