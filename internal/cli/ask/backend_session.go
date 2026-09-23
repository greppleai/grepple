package ask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/cliruntime"
)

const researchSessionCacheVersion = "grepple-ask-research-cache-v1"

type researchSession struct {
	application cliruntime.Context
	ctx         context.Context
	identity    string
	server      string
	log         *agent.Log
	universeMu  sync.Mutex
	universes   map[string]*localResearchUniverse
}

func newResearchSession(application cliruntime.Context, ctx context.Context, log *agent.Log, root, server string) *researchSession {
	if ctx == nil {
		ctx = context.Background()
	}
	return &researchSession{application: application, ctx: ctx, identity: researchSourceIdentity(application, root, server), server: server, log: log, universes: make(map[string]*localResearchUniverse)}
}

func (s *researchSession) localUniverse(globs []string, maxFiles int) (*localResearchUniverse, error) {
	plan, err := planLocalResearchUniverse(s.application, globs, maxFiles)
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

func researchSourceIdentity(application cliruntime.Context, root, server string) string {
	canonicalRoot, err := filepath.Abs(root)
	if err != nil {
		canonicalRoot = filepath.Clean(root)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(canonicalRoot); resolveErr == nil {
		canonicalRoot = resolved
	}
	scope, _ := application.Repository().ScopeOptions()
	invocation := application.Repository().InvocationOptions()
	identity, _ := json.Marshal(struct {
		Version     string   `json:"version"`
		Root        string   `json:"root"`
		IgnorePaths []string `json:"ignorePaths,omitempty"`
		Production  bool     `json:"productionOnly,omitempty"`
		NoConfig    bool     `json:"noRepositoryConfig,omitempty"`
		ScopeFlags  []string `json:"scopeFlags,omitempty"`
		Server      string   `json:"server"`
	}{researchSessionCacheVersion, canonicalRoot, scope.IgnorePaths, invocation.ProductionOnly, invocation.NoRepositoryConfig, application.Repository().AppendScopeFlags(nil), strings.TrimRight(server, "/")})
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:])
}
