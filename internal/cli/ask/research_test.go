package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
)

type testResearchBackend struct{}

func (testResearchBackend) Identity() string { return "test" }
func (testResearchBackend) Close()           {}
func (testResearchBackend) Search(context.Context, SearchInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) Navigate(context.Context, NavigateInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) Structural(context.Context, StructuralInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) Architecture(context.Context, ArchitectureInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) Graph(context.Context, GraphInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) SourceScope(context.Context, SourceScopeInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) RepositoryRefs(context.Context, RepositoryRefsInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) RepositoryTree(context.Context, RepositoryTreeInput) (any, error) {
	return map[string]any{}, nil
}
func (testResearchBackend) Read(context.Context, ReadInput) (fantasy.ToolResponse, error) {
	return fantasy.NewTextResponse("ok"), nil
}

func TestResearchToolsAreTypedAndDirect(t *testing.T) {
	tools := researchTools(newToolSession(context.Background(), nil, "test"), testResearchBackend{})
	info := researchToolInfo(tools)
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"grepple"`)) {
		t.Fatalf("generic CLI tool is exposed: %s", encoded)
	}
	for _, field := range []string{`"query"`, `"location"`, `"operation"`, `"direction"`, `"repository"`} {
		if !bytes.Contains(encoded, []byte(field)) {
			t.Fatalf("typed tool schemas missing %s: %s", field, encoded)
		}
	}
}

func TestResearchSessionCachesEquivalentDefaultInputs(t *testing.T) {
	session := newToolSession(context.Background(), nil, "test")
	calls := 0
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		calls++
		return fantasy.NewTextResponse("ok"), nil
	}
	first, err := session.run(context.Background(), "search_code", SearchInput{Query: "needle"}, execute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.run(context.Background(), "search_code", SearchInput{Query: "needle", Mode: "snippets", Limit: 8}, execute)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || first.Content != second.Content {
		t.Fatalf("calls=%d first=%q second=%q", calls, first.Content, second.Content)
	}
}

func TestResearchResultDisclosesTruncation(t *testing.T) {
	response, err := researchResult(strings.Repeat("x", researchToolOutputLimit+1), nil)
	if err != nil || response.IsError || !strings.Contains(response.Content, `"truncated":true`) {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestResearchSessionDoesNotCacheToolErrors(t *testing.T) {
	session := newToolSession(context.Background(), nil, "test")
	calls := 0
	execute := func(context.Context) (fantasy.ToolResponse, error) {
		calls++
		return fantasy.NewTextErrorResponse("failed"), nil
	}
	for range 2 {
		response, err := session.run(context.Background(), "search_code", SearchInput{Query: "needle"}, execute)
		if err != nil || !response.IsError {
			t.Fatalf("response=%+v err=%v", response, err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2", calls)
	}
}

func TestResearchSessionExecutesWithSessionContext(t *testing.T) {
	type contextKey struct{}
	sessionContext := context.WithValue(context.Background(), contextKey{}, "session")
	session := newToolSession(sessionContext, nil, "test")
	response, err := session.run(context.Background(), "search_code", SearchInput{Query: "needle"}, func(ctx context.Context) (fantasy.ToolResponse, error) {
		return fantasy.NewTextResponse(ctx.Value(contextKey{}).(string)), nil
	})
	if err != nil || response.Content != "session" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}
