package cli

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func reposTestServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(api.ReposResponse{
			OK:    true,
			Count: 3,
			Repos: []api.RepoListEntry{
				{Repo: "acme/api"},
				{Repo: "acme/web"},
				{Repo: "other/tools"},
			},
		})
	}))
	t.Cleanup(server.Close)
	return server, &gotPath
}

func TestReposListsNamesOnePerLine(t *testing.T) {
	isolateCLIAuth(t)
	server, gotPath := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := runRepos([]string{"--server", server.URL}); err != nil {
			t.Fatal(err)
		}
	})

	if *gotPath != "/public/repos" {
		t.Fatalf("expected GET /public/repos, got %s", *gotPath)
	}
	want := "acme/api\nacme/web\nother/tools\n"
	if out != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", out, want)
	}
}

func TestReposFilterSubstringCaseInsensitive(t *testing.T) {
	isolateCLIAuth(t)
	server, _ := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := runRepos([]string{"--server", server.URL, "ACME"}); err != nil {
			t.Fatal(err)
		}
	})

	if out != "acme/api\nacme/web\n" {
		t.Fatalf("filter mismatch: %q", out)
	}
}

func TestReposJSONMode(t *testing.T) {
	isolateCLIAuth(t)
	server, _ := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := runRepos([]string{"--server", server.URL, "--json", "web"}); err != nil {
			t.Fatal(err)
		}
	})

	var got api.ReposResponse
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if got.Count != 1 || len(got.Repos) != 1 || got.Repos[0].Repo != "acme/web" {
		t.Fatalf("unexpected filtered JSON: %#v", got)
	}
	if !strings.Contains(out, "\"repos\"") {
		t.Fatalf("expected repos key in JSON: %s", out)
	}
}
