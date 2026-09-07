package shard

import (
	"fmt"
	"grepple/internal/api"
	"grepple/internal/search"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestZoektQuery(t *testing.T) {
	tests := []struct {
		name string
		in   search.Params
		want string
	}{
		{name: "fixed", in: search.Params{Query: `hello "world"`, Regex: false}, want: `case:yes content:"hello \"world\""`},
		{name: "fixed with metacharacters", in: search.Params{Query: `a.b`, Regex: false}, want: `case:yes content:"a\\.b"`},
		{name: "case insensitive", in: search.Params{Query: "deploy", Regex: true, IgnoreCase: true}, want: `case:no content:"deploy"`},
		{name: "regex alternation", in: search.Params{Query: "(deploy|release)", Regex: true}, want: `case:yes content:"(deploy|release)"`},
		{name: "escaped regex", in: search.Params{Query: `console\.log`, Regex: true}, want: `case:yes content:"console\\.log"`},
		{name: "invalid regex", in: search.Params{Query: `(oops`, Regex: true}, want: ""},
		{name: "short fixed string", in: search.Params{Query: "go", Regex: false}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := zoektQuery(test.in); got != test.want {
				t.Fatalf("zoektQuery()=%q want %q", got, test.want)
			}
		})
	}
}

func TestReconcileAndRemoveZoektShards(t *testing.T) {
	indexDir := t.TempDir()
	for _, name := range []string{
		"owner%2Frepo_v16.00000.zoekt",
		"repo_v16.00000.zoekt",
		"other%2Frepo_v16.00000.zoekt",
		"README.txt",
	} {
		if err := os.WriteFile(filepath.Join(indexDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	options := zoektOptions{indexDir: indexDir}
	if err := reconcileZoektShards(options, []api.RepoInfo{{Repo: "owner/repo"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(indexDir, "owner%2Frepo_v16.00000.zoekt")); err != nil {
		t.Fatal("owned shard was removed")
	}
	if _, err := os.Stat(filepath.Join(indexDir, "repo_v16.00000.zoekt")); !os.IsNotExist(err) {
		t.Fatal("legacy basename shard was not removed")
	}
	if err := removeZoektRepo(options, "owner/repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(indexDir, "owner%2Frepo_v16.00000.zoekt")); !os.IsNotExist(err) {
		t.Fatal("repository shard was not removed")
	}
	longRepo := "owner/" + strings.Repeat("repository", 30)
	longShard := zoektShardPrefix(longRepo) + "_v16.00000.zoekt"
	if err := os.WriteFile(filepath.Join(indexDir, longShard), []byte("long"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := removeZoektRepo(options, longRepo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(indexDir, longShard)); !os.IsNotExist(err) {
		t.Fatal("long repository shard was not removed")
	}
}

func TestZoektCoverageDetection(t *testing.T) {
	repo := initGitRepo(t)
	writeRepoFile(t, repo, "sample.txt", "ordinary searchable content\n")
	runGit(t, repo, "add", "sample.txt")
	runGit(t, repo, "commit", "-qm", "sample")

	coverage := inspectZoektCoverage(repo)
	if !coverage.known || !coverage.indexed || len(coverage.fallback) != 0 {
		t.Fatalf("ordinary repository coverage=%#v", coverage)
	}

	// A .sourcegraph/ignore file forces tracked files into scanner fallback.
	writeRepoFile(t, repo, ".sourcegraph/ignore", "sample.txt\n")
	coverage = inspectZoektCoverage(repo)
	if !coverage.known || !slices.Contains(coverage.fallback, filepath.Join(repo, "sample.txt")) {
		t.Fatalf("sourcegraph-ignore coverage=%#v", coverage)
	}
	if err := os.RemoveAll(filepath.Join(repo, ".sourcegraph")); err != nil {
		t.Fatal(err)
	}

	// A file with too many distinct trigrams defeats the index.
	writeRepoFile(t, repo, "diverse.txt", highTrigramContent())
	runGit(t, repo, "add", "diverse.txt")
	coverage = inspectZoektCoverage(repo)
	if !coverage.known || !slices.Contains(coverage.fallback, filepath.Join(repo, "diverse.txt")) {
		t.Fatalf("high-trigram coverage=%#v", coverage)
	}
}

// initGitRepo creates a git repository in a temp dir with a test identity.
func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	return repo
}

// runGit executes one git command in the repository, failing the test on error.
func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	if output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

// writeRepoFile writes content to a file in the repository (creating parent
// directories), failing the test on error.
func writeRepoFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// highTrigramContent returns text with more than 20k distinct trigrams, which
// exceeds the indexable trigram-diversity cap.
func highTrigramContent() string {
	var diverse strings.Builder
	for first := rune('A'); first < 'A'+28; first++ {
		for second := rune('A'); second < 'A'+28; second++ {
			for third := rune('A'); third < 'A'+28; third++ {
				diverse.WriteRune(first)
				diverse.WriteRune(second)
				diverse.WriteRune(third)
			}
		}
	}
	return diverse.String()
}

func TestZoektCandidatesTruncationIsNotAnError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// FlushReason/FilesSkipped means Zoekt hit a result cap. grepple does not rank,
	// so the files it did return are valid matches: accept them and flag truncated
	// rather than discarding them and falling back to a full scan.
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"FlushReason":1,"FilesSkipped":42,"Files":[{"FileName":"main.go","Repository":"owner/repo"}]}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	paths, truncated, err := zoektCandidates(zoektOptions{port: port}, "deploy", t.TempDir(), []api.RepoInfo{{Repo: "owner/repo"}})
	if err != nil {
		t.Fatalf("truncation must not be an error: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true when Zoekt skipped files")
	}
	if len(paths) != 1 {
		t.Fatalf("expected the returned candidate to be kept, got %d", len(paths))
	}
}

func TestZoektCandidatesCrashIsAnError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// A crash means the index is unreliable, so surface an error and let the
	// caller fall back to a full filesystem scan.
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Crashes":1,"Files":[]}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if _, _, err := zoektCandidates(zoektOptions{port: port}, "deploy", t.TempDir(), []api.RepoInfo{{Repo: "owner/repo"}}); err == nil {
		t.Fatal("expected a Zoekt crash to be surfaced as an error")
	}
}

func TestZoektProcessAndCandidates(t *testing.T) {
	binDir := installZoektHelpers(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	root := t.TempDir()
	indexDir := t.TempDir()
	options := zoektOptions{binDir: binDir, indexDir: indexDir, repoRoot: root, port: port}
	if err := zoektIndex(options, filepath.Join(root, "owner", "repo")); err != nil {
		t.Fatal(err)
	}
	process, err := startZoekt(options)
	if err != nil {
		t.Fatal(err)
	}
	if !process.running() {
		t.Fatal("Zoekt process is not running")
	}
	// startZoekt no longer blocks on readiness; wait for the webserver here.
	waitZoektReady(options, process)

	paths, _, err := zoektCandidates(options, "case:no content:\"deploy\"", root, []api.RepoInfo{{Repo: "owner/repo"}, {Repo: "other/repo"}})
	if err != nil {
		process.stop()
		t.Fatal(err)
	}
	want := filepath.Join(root, "owner", "repo", "README.md")
	if !slices.Equal(paths, []string{want}) {
		process.stop()
		t.Fatalf("candidates=%q want [%q]", paths, want)
	}
	process.stop()
	if process.running() {
		t.Fatal("Zoekt process remained running after stop")
	}
}

func installZoektHelpers(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	t.Setenv("GO_WANT_ZOEKT_HELPER_PROCESS", "1")
	t.Setenv("GO_ZOEKT_TEST_BINARY", os.Args[0])
	for _, helper := range []struct {
		name, mode string
	}{{"zoekt-git-index", "index"}, {"zoekt-webserver", "web"}} {
		path := filepath.Join(binDir, helper.name)
		script := fmt.Sprintf("#!/bin/sh\nexec \"$GO_ZOEKT_TEST_BINARY\" -test.run=TestZoektHelperProcess -- %s \"$@\"\n", helper.mode)
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	return binDir
}

func TestZoektHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_ZOEKT_HELPER_PROCESS") != "1" {
		return
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	args := os.Args[separator+1:]
	switch args[0] {
	case "index":
		os.Exit(0)
	case "web":
		runZoektHelperWeb(args[1:])
	}
	os.Exit(2)
}

// runZoektHelperWeb serves the fake zoekt-webserver HTTP API on the -listen
// address: /api/search returns one canned file, everything else reports ok.
// It exits the helper process on misconfiguration or server failure.
func runZoektHelperWeb(args []string) {
	address := ""
	for index, arg := range args {
		if arg == "-listen" && index+1 < len(args) {
			address = args[index+1]
			break
		}
	}
	if address == "" {
		os.Exit(2)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/api/search") {
			_, _ = w.Write([]byte(`{"Result":{"Files":[{"FileName":"README.md","Repository":"owner/repo"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	if err := http.ListenAndServe(address, handler); err != nil {
		os.Exit(1)
	}
}
