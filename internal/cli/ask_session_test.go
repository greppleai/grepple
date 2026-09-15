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
	if len(lines) != 2 {
		t.Fatalf("cache log events=%d, want 2: %s", len(lines), content)
	}
	var statuses []askLogEvent
	for _, line := range lines {
		var event askLogEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, event)
	}
	if statuses[0].Type != "tool.cache" || statuses[1].Type != "tool.cache" || !strings.Contains(lines[1], `"hit":true`) {
		t.Fatalf("unexpected cache log: %s", content)
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
