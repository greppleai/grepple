package shard

import (
	"grepple/internal/api"
	"net"
	"net/http"
	"testing"
)

// TestZoektRepoCountsAggregatesFromIndex verifies counts are computed purely from
// the Zoekt response: files = matching files, matches = distinct matching lines,
// scoped to the allow-list, with per-repo aggregation and deterministic order.
func TestZoektRepoCountsAggregatesFromIndex(t *testing.T) {
	// owner/a: file1 matches lines {1,1,3} => 2 distinct lines; file2 line {5} => 1.
	// owner/b: file3 lines {2,4} => 2. other/x is not in the allow-list => ignored.
	body := `{"Result":{"FlushReason":0,"Crashes":0,"Files":[
		{"Repository":"owner/a","FileName":"file1","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":1}},{"Start":{"LineNumber":1}},{"Start":{"LineNumber":3}}]}]},
		{"Repository":"owner/a","FileName":"file2","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":5}}]}]},
		{"Repository":"owner/b","FileName":"file3","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":2}},{"Start":{"LineNumber":4}}]}]},
		{"Repository":"other/x","FileName":"file4","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":9}}]}]}
	]}}`

	port := serveZoekt(t, body)
	allow := []api.RepoInfo{{Repo: "owner/a"}, {Repo: "owner/b"}}
	counts, truncated, err := zoektRepoCounts(zoektOptions{port: port}, "class", allow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("no skip fields set => truncated must be false")
	}
	if len(counts) != 2 {
		t.Fatalf("expected 2 repos (other/x filtered), got %#v", counts)
	}
	// Deterministic order: matches desc. owner/a=3 (2+1), owner/b=2.
	if counts[0].Repo != "owner/a" || counts[0].Files != 2 || counts[0].Matches != 3 {
		t.Fatalf("owner/a should be files=2 matches=3, got %#v", counts[0])
	}
	if counts[1].Repo != "owner/b" || counts[1].Files != 1 || counts[1].Matches != 2 {
		t.Fatalf("owner/b should be files=1 matches=2, got %#v", counts[1])
	}
}

func TestZoektRepoCountsFlagsTruncation(t *testing.T) {
	port := serveZoekt(t, `{"Result":{"FilesSkipped":10,"Files":[
		{"Repository":"owner/a","FileName":"f","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":1}}]}]}
	]}}`)
	_, truncated, err := zoektRepoCounts(zoektOptions{port: port}, "class", []api.RepoInfo{{Repo: "owner/a"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("FilesSkipped>0 must set truncated")
	}
}

func TestZoektRepoCountsCrashIsError(t *testing.T) {
	port := serveZoekt(t, `{"Result":{"Crashes":1,"Files":[]}}`)
	if _, _, err := zoektRepoCounts(zoektOptions{port: port}, "class", []api.RepoInfo{{Repo: "owner/a"}}, nil); err == nil {
		t.Fatal("a crash must be surfaced as an error so the caller can fall back")
	}
}

// serveZoekt starts a fake Zoekt /api/search returning body and returns its port.
func serveZoekt(t *testing.T, body string) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(body))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return listener.Addr().(*net.TCPAddr).Port
}
