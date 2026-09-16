package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/greppleai/grepple/parser"
)

func TestResearchSessionCachesNormalizedSuccessfulResults(t *testing.T) {
	session := newResearchSession(context.Background(), nil, t.TempDir(), "https://example.invalid/")
	var calls atomic.Int64
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		calls.Add(1)
		return fantasy.NewTextResponse("evidence"), nil
	}

	first, err := session.run(context.Background(), "search_code", askSearchInput{Query: "Symbol"}, execute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.run(context.Background(), "search_code", askSearchInput{Query: "Symbol", Mode: "snippets", Limit: 8}, execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("executions=%d, want 1", calls.Load())
	}
	if first.Content != second.Content || first.Content != "evidence" {
		t.Fatalf("cold/warm evidence differs: %#v %#v", first, second)
	}
	if !strings.Contains(first.Metadata, `"cacheHit":false`) || !strings.Contains(second.Metadata, `"cacheHit":true`) {
		t.Fatalf("unexpected cache metadata: first=%q second=%q", first.Metadata, second.Metadata)
	}
}

func TestResearchSessionCoalescesConcurrentCalls(t *testing.T) {
	session := newResearchSession(context.Background(), nil, t.TempDir(), "")
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		calls.Add(1)
		close(entered)
		<-release
		return fantasy.NewTextResponse("shared evidence"), nil
	}
	type result struct {
		response fantasy.ToolResponse
		err      error
	}
	results := make(chan result, 2)
	go func() {
		response, err := session.run(context.Background(), "read_file", readToolInput{Path: "source.go"}, execute)
		results <- result{response, err}
	}()
	<-entered
	go func() {
		response, err := session.run(context.Background(), "read_file", readToolInput{Path: "source.go", StartLine: 1, EndLine: 200}, execute)
		results <- result{response, err}
	}()
	waitForResearchWaiter(t, session)
	close(release)

	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("errors: %v, %v", first.err, second.err)
	}
	if calls.Load() != 1 {
		t.Fatalf("executions=%d, want 1", calls.Load())
	}
	if first.response.Content != second.response.Content || first.response.Content != "shared evidence" {
		t.Fatalf("coalesced evidence differs: %#v %#v", first.response, second.response)
	}
	if !strings.Contains(first.response.Metadata+second.response.Metadata, `"cacheShared":true`) {
		t.Fatalf("one response must disclose shared work: %q %q", first.response.Metadata, second.response.Metadata)
	}
}

func TestResearchSessionDoesNotCacheErrors(t *testing.T) {
	session := newResearchSession(context.Background(), nil, t.TempDir(), "")
	var calls atomic.Int64
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		calls.Add(1)
		return fantasy.NewTextErrorResponse("temporary failure"), nil
	}
	for range 2 {
		response, err := session.run(context.Background(), "repository_tree", askRepositoryTreeInput{Repository: "owner/repo"}, execute)
		if err != nil {
			t.Fatal(err)
		}
		if !response.IsError {
			t.Fatalf("expected tool error response: %#v", response)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("error executions=%d, want 2", calls.Load())
	}
}

func TestResearchSessionCanceledWaiterDoesNotCancelLeader(t *testing.T) {
	session := newResearchSession(context.Background(), nil, t.TempDir(), "")
	entered := make(chan struct{})
	release := make(chan struct{})
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		close(entered)
		<-release
		return fantasy.NewTextResponse("complete"), nil
	}
	leader := make(chan error, 1)
	go func() {
		_, err := session.run(context.Background(), "query_graph", askGraphInput{Symbol: "Run"}, execute)
		leader <- err
	}()
	<-entered
	waiterContext, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() {
		_, err := session.run(waiterContext, "query_graph", askGraphInput{Symbol: "Run", Depth: 1}, execute)
		waiter <- err
	}()
	waitForResearchWaiter(t, session)
	cancel()
	if err := <-waiter; err != context.Canceled {
		t.Fatalf("waiter error=%v, want context canceled", err)
	}
	close(release)
	if err := <-leader; err != nil {
		t.Fatalf("leader was canceled: %v", err)
	}
}

func TestResearchSessionIdentityIncludesRootConfigScopeAndServer(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "grepple.json")
	if err := os.WriteFile(config, []byte(`{"ignore":{"paths":["tmp/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	first := researchSourceIdentity(root, "https://one.invalid/")
	if err := os.WriteFile(config, []byte(`{"ignore":{"paths":["generated/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	second := researchSourceIdentity(root, "https://one.invalid")
	third := researchSourceIdentity(root, "https://two.invalid")
	if first == second || second == third {
		t.Fatalf("identity did not change for config/server: %q %q %q", first, second, third)
	}
}

func TestResearchUniverseColdAndReusedOutputsMatch(t *testing.T) {
	root := t.TempDir()
	source := "package sample\n\nfunc Parse() { helper() }\nfunc helper() {}\n"
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	navigateInput := askNavigateInput{Location: "source.go:3", FollowDepth: 1}
	graphInput := askGraphInput{Direction: "callees", Symbol: "Parse"}
	architectureInput := askArchitectureInput{Operation: "resolve", Symbol: "Parse"}

	coldNavigation, err := runAskNavigate(context.Background(), root, "", navigateInput)
	if err != nil {
		t.Fatal(err)
	}
	coldGraph, err := runAskGraph(root, graphInput)
	if err != nil {
		t.Fatal(err)
	}
	coldArchitecture, err := runAskArchitecture(root, architectureInput)
	if err != nil {
		t.Fatal(err)
	}

	session := newResearchSession(context.Background(), nil, root, "")
	defer session.Close()
	reusedNavigation, err := runAskNavigateWithSession(context.Background(), session, root, "", navigateInput)
	if err != nil {
		t.Fatal(err)
	}
	reusedGraph, err := runAskGraphWithSession(session, root, graphInput)
	if err != nil {
		t.Fatal(err)
	}
	reusedArchitecture, err := runAskArchitectureWithSession(session, root, architectureInput)
	if err != nil {
		t.Fatal(err)
	}
	assertResearchJSONEqual(t, "navigation", coldNavigation, reusedNavigation)
	assertResearchJSONEqual(t, "graph", coldGraph, reusedGraph)
	assertResearchJSONEqual(t, "architecture", coldArchitecture, reusedArchitecture)
}

func TestResearchUniversePreservesIncompleteSourceMetadata(t *testing.T) {
	root := t.TempDir()
	mustWriteResearchFile(t, filepath.Join(root, "source.go"), "package sample\nfunc Parse() {}\n")
	mustWriteResearchFile(t, filepath.Join(root, "binary.go"), "package sample\x00\n")
	t.Chdir(root)
	expectedGraph, err := buildNavigationGraphOutput(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	expectedArchitecture, err := buildDirectoryArchitecture(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	session := newResearchSession(context.Background(), nil, root, "")
	defer session.Close()
	universe, err := session.localUniverse(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertResearchJSONEqual(t, "incomplete graph", expectedGraph, universe.navigationOutput())
	assertResearchJSONEqual(t, "incomplete architecture", expectedArchitecture, universe.architecture())
}

func assertResearchJSONEqual(t *testing.T, name string, left, right any) {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("%s cold/reused output differs:\n%s\n%s", name, leftJSON, rightJSON)
	}
}

func TestResearchSessionLogsCacheStatusAndPreservesToolEvidence(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(askLogDirectoryEnv, directory)
	log, err := newAskLog()
	if err != nil {
		t.Fatal(err)
	}
	session := newResearchSession(context.Background(), log, directory, "")
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse("same evidence"), nil
	}
	first, err := session.run(context.Background(), "explain_sources", askSourceScopeInput{}, execute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.run(context.Background(), "explain_sources", askSourceScopeInput{}, execute)
	if err != nil {
		t.Fatal(err)
	}
	if first.Content != second.Content {
		t.Fatalf("evidence changed on cache hit: %q != %q", first.Content, second.Content)
	}
	path := log.Path()
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 4 {
		t.Fatalf("cache/timing log events=%d, want 4: %s", len(lines), content)
	}
	var statuses []askLogEvent
	for _, line := range lines {
		var event askLogEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, event)
	}
	if statuses[0].Type != "tool.cache" || statuses[1].Type != "tool.timing" || statuses[2].Type != "tool.cache" || statuses[3].Type != "tool.timing" || !strings.Contains(lines[2], `"hit":true`) || !strings.Contains(lines[3], `"cacheHit":true`) {
		t.Fatalf("unexpected cache/timing log: %s", content)
	}
	if !strings.Contains(lines[1], `"input":{}`) || !strings.Contains(lines[1], `"durationMs":`) || !strings.Contains(lines[1], `"executionDurationMs":`) || !strings.Contains(lines[1], `"executed":true`) || !strings.Contains(lines[1], `"responseBytes":13`) || strings.Contains(lines[1], `"response":`) {
		t.Fatalf("tool timing must include input and metrics without response content: %s", lines[1])
	}
}

func TestResearchSessionReusesOneUniverseAcrossAnalysisTools(t *testing.T) {
	root := t.TempDir()
	source := "package sample\n\nfunc Parse() { helper() }\nfunc helper() {}\n"
	mustWriteResearchFile(t, filepath.Join(root, "source.go"), source)
	t.Chdir(root)
	t.Setenv(askLogDirectoryEnv, t.TempDir())
	log := mustResearchLog(t)
	session := newResearchSession(context.Background(), log, root, "")
	researchUniverseBuilds.Store(0)

	navigation, err := runAskNavigateWithSession(context.Background(), session, root, "", askNavigateInput{Location: "source.go:3", FollowDepth: 1})
	assertResearchResult(t, "navigation", navigation, err, "helper")
	graph, err := runAskGraphWithSession(session, root, askGraphInput{Direction: "callees", Symbol: "Parse"})
	assertResearchResult(t, "graph", graph, err, "helper")
	architecture, err := runAskArchitectureWithSession(session, root, askArchitectureInput{Operation: "resolve", Symbol: "Parse"})
	assertResearchResult(t, "architecture", architecture, err, "Parse")
	if builds := researchUniverseBuilds.Load(); builds != 1 {
		t.Fatalf("research universe builds=%d, want 1", builds)
	}
	universe, err := session.localUniverse(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	document := universe.document(filepath.Join(root, "source.go"))
	if document == nil || document.Source() == "" {
		t.Fatal("expected live caller-owned document")
	}
	session.Close()
	assertResearchUniverseClosedAndLogged(t, log, document)
}

func TestResearchSessionNormalizesEquivalentUniverseScopes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package sample\nfunc "+strings.TrimSuffix(name, ".go")+"() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	session := newResearchSession(context.Background(), nil, root, "")
	defer session.Close()
	researchUniverseBuilds.Store(0)
	first, err := session.localUniverse([]string{"b.go", "a.go"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.localUniverse([]string{"a.go", "b.go"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || researchUniverseBuilds.Load() != 1 {
		t.Fatalf("equivalent scopes did not share one universe: same=%v builds=%d", first == second, researchUniverseBuilds.Load())
	}
}

func mustWriteResearchFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustResearchLog(t *testing.T) *askLog {
	t.Helper()
	log, err := newAskLog()
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func assertResearchResult(t *testing.T, name string, value any, err error, expected string) {
	t.Helper()
	if err != nil || !jsonContains(value, expected) {
		t.Fatalf("%s=%+v err=%v", name, value, err)
	}
}

func assertResearchUniverseClosedAndLogged(t *testing.T, log *askLog, document *parser.Document) {
	t.Helper()
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	logContent, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	if events := strings.Count(string(logContent), `"type":"research.universe"`); events != 4 {
		t.Fatalf("research universe log events=%d, want 4: %s", events, logContent)
	}
	if reused := strings.Count(string(logContent), `"reused":true`); reused != 3 {
		t.Fatalf("reused universe events=%d, want 3: %s", reused, logContent)
	}
	if document.Source() != "" {
		t.Fatal("session close did not release its document")
	}
}

func waitForResearchWaiter(t *testing.T, session *researchSession) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		waiting := false
		for _, pending := range session.inflight {
			waiting = pending.waiters > 0
		}
		session.mu.Unlock()
		if waiting {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("duplicate call did not join in-flight work")
}
