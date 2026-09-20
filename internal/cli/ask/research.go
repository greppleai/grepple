package ask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/greppleai/grepple/internal/agent"
	agentresearch "github.com/greppleai/grepple/internal/agent/research"
	"github.com/greppleai/grepple/internal/aiprovider"
)

const researchToolOutputLimit = 64 << 10

// SearchInput describes one source-text research request.
type SearchInput struct {
	Query      string   `json:"query" description:"Literal text to find, or a regular expression when regex is true"`
	Paths      []string `json:"paths,omitempty" description:"Local repository-relative files, directories, or globs; defaults to the workspace"`
	Repository string   `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local search"`
	Regex      bool     `json:"regex,omitempty"`
	IgnoreCase bool     `json:"ignore_case,omitempty"`
	Mode       string   `json:"mode,omitempty" description:"Result shape: snippets (default), files, or count"`
	Limit      int      `json:"limit,omitempty"`
	Context    int      `json:"context,omitempty"`
}

// NavigateInput describes one declaration navigation request.
type NavigateInput struct {
	Location    string `json:"location"`
	Repository  string `json:"repository,omitempty"`
	FollowDepth int    `json:"follow_depth,omitempty"`
}

// StructuralInput describes one structural-query request.
type StructuralInput struct {
	Query      string   `json:"query"`
	Paths      []string `json:"paths,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Limit      int      `json:"limit,omitempty"`
}

// ArchitectureInput describes one architecture operation.
type ArchitectureInput struct {
	Operation  string   `json:"operation"`
	Paths      []string `json:"paths,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Symbol     string   `json:"symbol,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	MaxFiles   int      `json:"max_files,omitempty"`
}

// GraphInput describes one navigation-graph query.
type GraphInput struct {
	Direction  string   `json:"direction"`
	Symbol     string   `json:"symbol,omitempty"`
	Location   string   `json:"location,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Depth      int      `json:"depth,omitempty"`
	Language   string   `json:"language,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
}

// SourceScopeInput describes source classification scope.
type SourceScopeInput struct {
	Paths []string `json:"paths,omitempty"`
}

// RepositoryRefsInput selects indexed repository references.
type RepositoryRefsInput struct {
	Repository string `json:"repository"`
	Kind       string `json:"kind,omitempty"`
}

// RepositoryTreeInput selects one bounded indexed repository tree.
type RepositoryTreeInput struct {
	Repository string `json:"repository"`
	Path       string `json:"path,omitempty"`
	Depth      int    `json:"depth,omitempty"`
}

// ReadInput describes one local or indexed source read.
type ReadInput struct {
	Path       string      `json:"path,omitempty"`
	Files      []ReadRange `json:"files,omitempty"`
	Repository string      `json:"repository,omitempty"`
	StartLine  int         `json:"start_line,omitempty"`
	EndLine    int         `json:"end_line,omitempty"`
	Outline    bool        `json:"outline,omitempty"`
}

// ReadRange describes one entry in a batch source read.
type ReadRange struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Outline   bool   `json:"outline,omitempty"`
}

// ResearchBackend supplies repository operations without coupling this command to its parent package.
type ResearchBackend interface {
	Identity() string
	Search(context.Context, SearchInput) (any, error)
	Navigate(context.Context, NavigateInput) (any, error)
	Structural(context.Context, StructuralInput) (any, error)
	Architecture(context.Context, ArchitectureInput) (any, error)
	Graph(context.Context, GraphInput) (any, error)
	SourceScope(context.Context, SourceScopeInput) (any, error)
	RepositoryRefs(context.Context, RepositoryRefsInput) (any, error)
	RepositoryTree(context.Context, RepositoryTreeInput) (any, error)
	Read(context.Context, ReadInput) (fantasy.ToolResponse, error)
	Close()
}

type researchSession struct {
	ctx       context.Context
	log       *agent.Log
	telemetry *agent.Telemetry
	identity  string
	mu        sync.Mutex
	values    map[string]fantasy.ToolResponse
	inflight  map[string]*researchCall
}

type researchCall struct {
	done     chan struct{}
	response fantasy.ToolResponse
	err      error
}

func newResearchSession(ctx context.Context, log *agent.Log, identity string) *researchSession {
	return &researchSession{ctx: ctx, log: log, telemetry: agent.NewTelemetry(time.Now()), identity: identity, values: make(map[string]fantasy.ToolResponse), inflight: make(map[string]*researchCall)}
}

func normalizeResearchInput(input any) any {
	switch value := input.(type) {
	case SearchInput:
		return normalizeSearchInput(value)
	case StructuralInput:
		if value.Limit == 0 {
			value.Limit = 10
		}
		return value
	case GraphInput:
		if value.Depth == 0 {
			value.Depth = 1
		}
		return value
	case RepositoryTreeInput:
		if value.Depth == 0 {
			value.Depth = 2
		}
		return value
	case ReadInput:
		return normalizeReadInput(value)
	default:
		return input
	}
}

func normalizeSearchInput(value SearchInput) SearchInput {
	if value.Mode == "" {
		value.Mode = "snippets"
	}
	if value.Limit == 0 {
		value.Limit = 8
	}
	value.Context = min(value.Context, 3)
	return value
}

func normalizeReadInput(value ReadInput) ReadInput {
	if len(value.Files) != 0 {
		for index := range value.Files {
			value.Files[index] = normalizeReadRange(value.Files[index])
		}
		return value
	}
	if value.Outline {
		value.StartLine, value.EndLine = 0, 0
		return value
	}
	if value.StartLine < 1 {
		value.StartLine = 1
	}
	if value.EndLine == 0 {
		value.EndLine = value.StartLine + 199
	}
	return value
}

func normalizeReadRange(value ReadRange) ReadRange {
	if value.Outline {
		value.StartLine, value.EndLine = 0, 0
		return value
	}
	if value.StartLine < 1 {
		value.StartLine = 1
	}
	if value.EndLine == 0 {
		value.EndLine = value.StartLine + 199
	}
	return value
}

type researchAcquireResult struct {
	response fantasy.ToolResponse
	pending  *researchCall
	ready    bool
	shared   bool
	err      error
}

func (s *researchSession) acquire(ctx context.Context, key string) researchAcquireResult {
	s.mu.Lock()
	if value, ok := s.values[key]; ok {
		s.mu.Unlock()
		return researchAcquireResult{response: value, ready: true}
	}
	if pending := s.inflight[key]; pending != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return researchAcquireResult{ready: true, shared: true, err: ctx.Err()}
		case <-pending.done:
			return researchAcquireResult{response: pending.response, ready: true, shared: true, err: pending.err}
		}
	}
	pending := &researchCall{done: make(chan struct{})}
	s.inflight[key] = pending
	s.mu.Unlock()
	return researchAcquireResult{pending: pending}
}
func (s *researchSession) run(ctx context.Context, name string, input any, execute func(context.Context) (fantasy.ToolResponse, error), calls ...fantasy.ToolCall) (result fantasy.ToolResponse, resultErr error) {
	input = normalizeResearchInput(input)
	encoded, err := json.Marshal(input)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	digest := sha256.Sum256(append([]byte(s.identity+"\x00"+name+"\x00"), encoded...))
	key := hex.EncodeToString(digest[:])
	callID := ""
	if len(calls) > 0 {
		callID = calls[0].ID
	}
	measurement := s.telemetry.BeginTool(name, input, time.Now(), callID)
	status := agent.CacheStatus{Tool: name, Key: key[:12]}
	defer func() {
		timing := s.telemetry.FinishTool(measurement, time.Now(), result, resultErr, status)
		if s.log != nil {
			resultErr = errors.Join(resultErr, s.log.Record("tool.timing", timing))
		}
	}()

	acquired := s.acquire(ctx, key)
	if acquired.ready {
		status.Hit = true
		status.Shared = acquired.shared
		if s.log != nil {
			_ = s.log.Record("tool.cache", status)
		}
		return acquired.response, acquired.err
	}
	pending := acquired.pending

	status.Executed = true
	s.telemetry.StartToolExecution(measurement, time.Now())
	result, resultErr = execute(s.ctx)
	s.mu.Lock()
	delete(s.inflight, key)
	pending.response, pending.err = result, resultErr
	if resultErr == nil && !result.IsError {
		s.values[key] = result
	}
	close(pending.done)
	s.mu.Unlock()
	if s.log != nil {
		_ = s.log.Record("tool.cache", status)
	}
	return result, resultErr
}

func cachedResearchTool[I any](session *researchSession, name, description string, run func(context.Context, I) (fantasy.ToolResponse, error)) fantasy.AgentTool {
	return fantasy.NewAgentTool(name, description, func(ctx context.Context, input I, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return session.run(ctx, name, input, func(runCtx context.Context) (fantasy.ToolResponse, error) { return run(runCtx, input) }, call)
	})
}

func researchResult(value any, err error) (fantasy.ToolResponse, error) {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(encoded) > researchToolOutputLimit {
		prefix := string(encoded[:researchToolOutputLimit/2])
		encoded, _ = json.Marshal(map[string]any{"truncated": true, "message": "result exceeded 64 KiB; narrow scope, limit, depth, or paths", "jsonPrefix": prefix})
	}
	return fantasy.NewTextResponse(string(encoded)), nil
}

func researchTools(session *researchSession, backend ResearchBackend) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		cachedResearchTool(session, "search_code", "Search source text directly. Use count, files, then snippets for bounded evidence.", func(ctx context.Context, input SearchInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.Search(ctx, input))
		}),
		cachedResearchTool(session, "navigate_code", "Retrieve the exact declaration at PATH:LINE and immediate callers and callees.", func(ctx context.Context, input NavigateInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.Navigate(ctx, input))
		}),
		cachedResearchTool(session, "structural_search", "Run a native read-only gritql-v1 syntax query.", func(ctx context.Context, input StructuralInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.Structural(ctx, input))
		}),
		cachedResearchTool(session, "inspect_architecture", "Inspect directory architecture, resolve symbols, relations, or responsibilities.", func(ctx context.Context, input ArchitectureInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.Architecture(ctx, input))
		}),
		cachedResearchTool(session, "query_graph", "Query the parser-owned graph for callers, callees, dependencies, dependents, or impact.", func(ctx context.Context, input GraphInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.Graph(ctx, input))
		}),
		cachedResearchTool(session, "explain_sources", "Report selected, excluded, ignored, generated, vendored, test, fixture, and production files.", func(ctx context.Context, input SourceScopeInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.SourceScope(ctx, input))
		}),
		cachedResearchTool(session, "repository_refs", "Resolve a repository and requested version to exact indexed selectors.", func(ctx context.Context, input RepositoryRefsInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.RepositoryRefs(ctx, input))
		}),
		cachedResearchTool(session, "repository_tree", "List a bounded tree from one exact indexed repository selector.", func(ctx context.Context, input RepositoryTreeInput) (fantasy.ToolResponse, error) {
			return researchResult(backend.RepositoryTree(ctx, input))
		}),
		cachedResearchTool(session, "read_file", "Batch-read up to eight bounded local or indexed-repository ranges.", backend.Read),
	}
}

func researchToolInfo(tools []fantasy.AgentTool) []fantasy.ToolInfo {
	result := make([]fantasy.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		result = append(result, tool.Info())
	}
	return result
}

// RunResearch executes one source-research agent session.
func RunResearch(ctx context.Context, log *agent.Log, provider aiprovider.Provider, request SessionRequest, backend ResearchBackend) (answer string, returnErr error) {
	session := newResearchSession(ctx, log, backend.Identity())
	defer func() {
		backend.Close()
		returnErr = errors.Join(returnErr, log.Record("session.performance", session.telemetry.Performance(time.Now())))
	}()
	prompt := agentresearch.SystemPrompt(request.Root)
	tools := researchTools(session, backend)
	if err := log.Record("session.start", map[string]any{"provider": provider.Name(), "model": request.Model, "question": request.Question, "root": request.Root, "server": request.Server, "timeoutSeconds": request.Timeout, "systemPrompt": prompt, "tools": researchToolInfo(tools)}); err != nil {
		return "", err
	}
	result, err := agent.Run(ctx, log, agent.Request{Name: "ask", Prompt: request.Question, SystemPrompt: prompt, Provider: provider.Name(), Model: request.Model, ModelFactory: func(ctx context.Context) (fantasy.LanguageModel, error) {
		return provider.LanguageModel(ctx, request.Model)
	}, Tools: tools, Telemetry: session.telemetry})
	if err != nil {
		return "", err
	}
	return result.Answer, nil
}
