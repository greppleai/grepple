package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestRefsListsSelectorsAndResolvedCommits(t *testing.T) {
	isolateCLIAuth(t)
	head := "abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(api.ReposResponse{OK: true, Count: 2, Repos: []api.RepoListEntry{
			{Repo: "owner/repo", Selector: "owner/repo", RefKind: "default", Ref: "main", Head: &head},
			{Repo: "owner/repo", Selector: "owner/repo@tag~v1.2.3", RefKind: "tag", Ref: "v1.2.3", Head: &head},
		}})
	}))
	defer server.Close()
	out := captureStdout(t, func() {
		if err := runRefs([]string{"--server", server.URL, "owner/repo"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "owner/repo@tag~v1.2.3\ttag\tv1.2.3\tabcdef") {
		t.Fatalf("missing indexed tag: %q", out)
	}
}

func TestRefsFiltersBySourceRepository(t *testing.T) {
	entries := []api.RepoListEntry{{Repo: "owner/repo"}, {Repo: "other/repo"}}
	filtered := refsForRepository(entries, "owner/repo", "")
	if len(filtered) != 1 || filtered[0].Repo != "owner/repo" {
		t.Fatalf("unexpected refs: %#v", filtered)
	}
}

func TestRefsFiltersTags(t *testing.T) {
	entries := []api.RepoListEntry{{Repo: "owner/repo", RefKind: "default"}, {Repo: "owner/repo", RefKind: "tag"}}
	filtered := refsForRepository(entries, "owner/repo", "tag")
	if len(filtered) != 1 || filtered[0].RefKind != "tag" {
		t.Fatalf("unexpected refs: %#v", filtered)
	}
}
