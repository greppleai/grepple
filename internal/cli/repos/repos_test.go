package repos

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
			Count: 4,
			Repos: []api.RepoListEntry{
				{Repo: "acme/api"},
				{Repo: "acme/api", Selector: "acme/api@tag~v1.0.0", RefKind: "tag", Ref: "v1.0.0"},
				{Repo: "acme/web"},
				{Repo: "other/tools"},
			},
		})
	}))
	t.Cleanup(server.Close)
	return server, &gotPath
}

func TestReposListsNamesOnePerLine(t *testing.T) {
	server, gotPath := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := Run([]string{"--server", server.URL}, Dependencies{}); err != nil {
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
	server, _ := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := Run([]string{"--server", server.URL, "ACME"}, Dependencies{}); err != nil {
			t.Fatal(err)
		}
	})

	if out != "acme/api\nacme/web\n" {
		t.Fatalf("filter mismatch: %q", out)
	}
}

func TestReposJSONMode(t *testing.T) {
	server, _ := reposTestServer(t)

	out := captureStdout(t, func() {
		if err := Run([]string{"--server", server.URL, "--json", "web"}, Dependencies{}); err != nil {
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
