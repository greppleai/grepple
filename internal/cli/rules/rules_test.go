package rules

import (
	"encoding/json"
	"github.com/greppleai/grepple/internal/wire"
	"net/http"
	"net/http/httptest"
	"os"
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
		gotRule   wire.Rule
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotRule)
		gotRule.ID = "uses-checkout"
		gotRule.Mode = wire.RuleModeCount
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
		_ = json.NewEncoder(w).Encode(wire.RuleResults{Rule: "r1", Mode: wire.RuleModeCount, Repos: []wire.RuleRepoResult{
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
		_ = json.NewEncoder(w).Encode(wire.RuleSet{Generation: 2, Rules: []wire.Rule{
			{ID: "a", Mode: wire.RuleModeCount, Name: "Alpha"},
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

func TestRulesAddPostsStructuralRule(t *testing.T) {
	isolateCLIAuth(t)
	queryPath := t.TempDir() + "/rule.grit"
	if err := os.WriteFile(queryPath, []byte("language go\n`target($x)`"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got wire.Rule
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		got.ID = "calls"
		got.Mode = wire.RuleModeFiles
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(got)
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		err := runRules([]string{"add", "--server", server.URL, "--grit", "--files", "--query-file", queryPath, "--repo", "owner/*", "**/*.go"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if got.Engine != wire.RuleEngineGritQL || got.Structural == nil {
		t.Fatalf("structural rule not sent: %#v", got)
	}
	if got.Structural.Query != "language go\n`target($x)`" || got.Structural.Compatibility != wire.GritCompatibilityV1 {
		t.Fatalf("structural query not sent: %#v", got.Structural)
	}
	if len(got.Structural.Repositories) != 1 || got.Structural.Repositories[0] != "owner/*" || len(got.Structural.Globs) != 1 {
		t.Fatalf("structural scope not sent: %#v", got.Structural)
	}
	if !strings.Contains(out, "created structural rule calls") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRulesAddRejectsInvalidStructuralQueryBeforeTransport(t *testing.T) {
	isolateCLIAuth(t)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	err := runRules([]string{"add", "--server", server.URL, "--engine", "gritql", "language go\n`unterminated"})
	if err == nil {
		t.Fatal("expected invalid structural query to fail")
	}
	if called {
		t.Fatal("invalid structural rule reached the server")
	}
}

func TestRulesListLabelsStructuralRules(t *testing.T) {
	isolateCLIAuth(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(wire.RuleSet{Rules: []wire.Rule{{ID: "calls", Mode: wire.RuleModeCount, Engine: wire.RuleEngineGritQL, Name: "Calls"}}})
	}))
	defer server.Close()
	out := captureStdout(t, func() {
		if err := runRules([]string{"list", "--server", server.URL}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "calls\tcount\tgritql\tCalls") {
		t.Fatalf("structural engine not rendered: %q", out)
	}
}
