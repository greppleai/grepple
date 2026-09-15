package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/fantasy"
)

const researchSessionCacheVersion = "grepple-ask-research-cache-v1"

type researchSession struct {
	ctx      context.Context
	identity string
	log      *askLog

	mu       sync.Mutex
	values   map[string]fantasy.ToolResponse
	inflight map[string]*researchPendingCall

	universeMu sync.Mutex
	universes  map[string]*localResearchUniverse
}

type researchPendingCall struct {
	done     chan struct{}
	waiters  int
	response fantasy.ToolResponse
	err      error
}

type researchCacheStatus struct {
	Tool   string `json:"tool"`
	Key    string `json:"key"`
	Hit    bool   `json:"hit"`
	Shared bool   `json:"shared"`
}

func newResearchSession(ctx context.Context, log *askLog, root, server string) *researchSession {
	if ctx == nil {
		ctx = context.Background()
	}
	return &researchSession{
		ctx:       ctx,
		identity:  researchSourceIdentity(root, server),
		log:       log,
		values:    make(map[string]fantasy.ToolResponse),
		inflight:  make(map[string]*researchPendingCall),
		universes: make(map[string]*localResearchUniverse),
	}
}

func (s *researchSession) localUniverse(globs []string, maxFiles int) (*localResearchUniverse, error) {
	plan, err := planLocalResearchUniverse(globs, maxFiles)
	if err != nil {
		return nil, err
	}
	s.universeMu.Lock()
	universe := s.universes[plan.key]
	created := universe == nil
	if created {
		universe = &localResearchUniverse{plan: plan}
		s.universes[plan.key] = universe
	}
	s.universeMu.Unlock()
	universe.once.Do(universe.load)
	if s.log != nil {
		if err := s.log.Record("research.universe", map[string]any{"key": plan.key[:12], "files": len(plan.paths), "reused": !created}); err != nil {
			return nil, err
		}
	}
	return universe, universe.err
}

// Close releases parser documents owned by this invocation after every tool has finished.
func (s *researchSession) Close() {
	s.universeMu.Lock()
	universes := make([]*localResearchUniverse, 0, len(s.universes))
	for _, universe := range s.universes {
		universes = append(universes, universe)
	}
	s.universeMu.Unlock()
	for _, universe := range universes {
		universe.close()
	}
}

func researchSourceIdentity(root, server string) string {
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		canonicalRoot = filepath.Clean(root)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonicalRoot); resolveErr == nil {
		canonicalRoot = resolved
	}
	configPath := ""
	configDigest := ""
	if !activeRepositoryOptions.disabled {
		if path, found := findRepositoryConfig(canonicalRoot); found {
			configPath = filepath.Clean(path)
			if digest, digestErr := fileSHA256(path); digestErr != nil {
				configDigest = "error:" + digestErr.Error()
			} else {
				configDigest = digest
			}
		}
	}
	identity, _ := json.Marshal(struct {
		Version      string   `json:"version"`
		Root         string   `json:"root"`
		ConfigPath   string   `json:"configPath,omitempty"`
		ConfigDigest string   `json:"configDigest,omitempty"`
		ScopeFlags   []string `json:"scopeFlags,omitempty"`
		Server       string   `json:"server"`
	}{researchSessionCacheVersion, canonicalRoot, configPath, configDigest, activeRepositoryScopeFlags(), strings.TrimRight(server, "/")})
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:])
}

func normalizeResearchInput(input any) any {
	switch value := input.(type) {
	case askSearchInput:
		return normalizeResearchSearchInput(value)
	case askStructuralInput:
		value.Limit = researchDefault(value.Limit, 10)
		return value
	case askGraphInput:
		value.Depth = researchDefault(value.Depth, 1)
		return value
	case askRepositoryTreeInput:
		value.Depth = researchDefault(value.Depth, 2)
		return value
	case readToolInput:
		return normalizeResearchReadInput(value)
	default:
		return input
	}
}

func normalizeResearchSearchInput(value askSearchInput) askSearchInput {
	if value.Mode == "" {
		value.Mode = "snippets"
	}
	value.Limit = researchDefault(value.Limit, 8)
	value.Context = min(value.Context, 3)
	return value
}

func normalizeResearchReadInput(value readToolInput) readToolInput {
	if value.Outline {
		value.StartLine = 0
		value.EndLine = 0
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

func researchDefault(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func (s *researchSession) cacheKey(tool string, input any) (string, error) {
	input = normalizeResearchInput(input)
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode %s cache input: %w", tool, err)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(researchSessionCacheVersion))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(s.identity))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(tool))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (s *researchSession) run(ctx context.Context, tool string, input any, execute func(context.Context) (fantasy.ToolResponse, error)) (fantasy.ToolResponse, error) {
	key, err := s.cacheKey(tool, input)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}

	s.mu.Lock()
	if response, ok := s.values[key]; ok {
		s.mu.Unlock()
		return s.finish(response, nil, researchCacheStatus{Tool: tool, Key: key[:12], Hit: true})
	}
	if pending, ok := s.inflight[key]; ok {
		pending.waiters++
		s.mu.Unlock()
		select {
		case <-pending.done:
			return s.finish(pending.response, pending.err, researchCacheStatus{Tool: tool, Key: key[:12], Hit: true, Shared: true})
		case <-ctx.Done():
			return fantasy.ToolResponse{}, ctx.Err()
		}
	}
	pending := &researchPendingCall{done: make(chan struct{})}
	s.inflight[key] = pending
	s.mu.Unlock()

	response, runErr := execute(s.ctx)
	s.mu.Lock()
	pending.response = response
	pending.err = runErr
	if runErr == nil && !response.IsError {
		s.values[key] = response
	}
	delete(s.inflight, key)
	close(pending.done)
	s.mu.Unlock()
	return s.finish(response, runErr, researchCacheStatus{Tool: tool, Key: key[:12]})
}

func (s *researchSession) finish(response fantasy.ToolResponse, runErr error, status researchCacheStatus) (fantasy.ToolResponse, error) {
	if s.log != nil {
		if err := s.log.Record("tool.cache", status); err != nil {
			return fantasy.ToolResponse{}, err
		}
	}
	response = fantasy.WithResponseMetadata(response, map[string]any{
		"cacheHit":    status.Hit,
		"cacheShared": status.Shared,
		"cacheKey":    status.Key,
	})
	return response, runErr
}

func cachedAskTool[T any](session *researchSession, name, description string, execute func(context.Context, T) (fantasy.ToolResponse, error)) fantasy.AgentTool {
	return fantasy.NewAgentTool(name, description, func(ctx context.Context, input T, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return session.run(ctx, name, input, func(runCtx context.Context) (fantasy.ToolResponse, error) {
			return execute(runCtx, input)
		})
	})
}
