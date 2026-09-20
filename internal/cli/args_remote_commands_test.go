package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestRunDispatchesReposAndTreeCommands(t *testing.T) {
	isolateCLIAuth(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/public/repos":
			_ = json.NewEncoder(writer).Encode(api.ReposResponse{OK: true, Count: 1, Repos: []api.RepoListEntry{{Repo: "owner/repo"}}})
		case "/public/tree":
			_ = json.NewEncoder(writer).Encode(api.TreeResponse{Repo: "owner/repo", Entries: []api.TreeEntry{{Path: "src/main.go"}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	reposOutput := captureStdout(t, func() {
		if err := Run([]string{"repos", "--server", server.URL}); err != nil {
			t.Fatal(err)
		}
	})
	if reposOutput != "owner/repo\n" {
		t.Fatalf("repos output=%q", reposOutput)
	}

	treeOutput := captureStdout(t, func() {
		if err := Run([]string{"tree", "--server", server.URL, "owner/repo"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(treeOutput, "owner/repo\n") || !strings.Contains(treeOutput, "main.go") {
		t.Fatalf("tree output=%q", treeOutput)
	}
}
