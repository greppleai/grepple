package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"grepple/internal/api"
	"grepple/internal/search"

	"go.uber.org/zap"
)

// ruleRegistry is the router's source of truth for predefined-grep definitions.
// It persists them to rules.json and hands a generation-stamped api.RuleSet to the
// shards, which materialize the results. The router holds no results itself; it
// aggregates them from the shards on read.
type ruleRegistry struct {
	mu         sync.RWMutex
	generation int64
	rules      map[string]api.Rule
	path       string
	o          routerOptions
	log        *zap.Logger
}

type ruleRegistryFile struct {
	Generation int64      `json:"generation"`
	Rules      []api.Rule `json:"rules"`
}

func newRuleRegistry(path string, o routerOptions, logger *zap.Logger) *ruleRegistry {
	if logger == nil {
		logger = zap.NewNop()
	}
	r := &ruleRegistry{rules: map[string]api.Rule{}, path: path, o: o, log: logger}
	r.load()
	return r
}

func (r *ruleRegistry) load() {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return
	}
	var file ruleRegistryFile
	if json.Unmarshal(data, &file) != nil {
		return
	}
	r.generation = file.Generation
	for _, rule := range file.Rules {
		if rule.ID != "" {
			r.rules[rule.ID] = rule
		}
	}
}

func (r *ruleRegistry) save() {
	r.mu.RLock()
	file := ruleRegistryFile{Generation: r.generation}
	for _, rule := range r.rules {
		file.Rules = append(file.Rules, rule)
	}
	r.mu.RUnlock()
	sort.Slice(file.Rules, func(i, j int) bool { return file.Rules[i].ID < file.Rules[j].ID })
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return
	}
	tmp := r.path + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, r.path)
	}
}

func (r *ruleRegistry) list() []api.Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]api.Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *ruleRegistry) get(id string) (api.Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.rules[id]
	return rule, ok
}

func (r *ruleRegistry) ruleSet() api.RuleSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := api.RuleSet{Generation: r.generation}
	for _, rule := range r.rules {
		set.Rules = append(set.Rules, rule)
	}
	sort.Slice(set.Rules, func(i, j int) bool { return set.Rules[i].ID < set.Rules[j].ID })
	return set
}

// upsert validates and stores a rule (create or replace), bumps the generation,
// and persists. The caller distributes the new set to the shards.
func (r *ruleRegistry) upsert(rule api.Rule) (api.Rule, error) {
	normalized, err := search.NormalizeRule(rule)
	if err != nil {
		return api.Rule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	r.mu.Lock()
	if existing, ok := r.rules[normalized.ID]; ok && existing.CreatedAt != "" {
		normalized.CreatedAt = existing.CreatedAt
	} else {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now
	r.rules[normalized.ID] = normalized
	r.generation++
	r.mu.Unlock()
	r.save()
	return normalized, nil
}

// remove deletes a rule and bumps the generation. Returns whether it existed.
func (r *ruleRegistry) remove(id string) bool {
	r.mu.Lock()
	_, ok := r.rules[id]
	if ok {
		delete(r.rules, id)
		r.generation++
	}
	r.mu.Unlock()
	if ok {
		r.save()
	}
	return ok
}

// distribute pushes the current ruleset to every shard (best-effort, in
// parallel). Shards apply idempotently by generation, so re-pushing is safe and
// lets a shard that was down catch up on the next call.
func (r *ruleRegistry) distribute() {
	set := r.ruleSet()
	body, err := json.Marshal(set)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, backend := range r.o.backends {
		wg.Add(1)
		go func(base string) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPut, base+"/rules", bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("content-type", "application/json")
			resp, err := (&http.Client{Timeout: r.o.timeout}).Do(req)
			if err != nil {
				r.log.Warn("rule distribution failed", zap.String("backend", shardName(base)), zap.Error(err))
				return
			}
			_ = resp.Body.Close()
		}(backend)
	}
	wg.Wait()
}

// recoverFromBackends adopts the highest-generation ruleset held by any shard
// when the router has none of its own (e.g. after a restart without a persisted
// rules.json). The shards keep the definitions, so this makes the router's state
// durable even without a mounted volume.
func (r *ruleRegistry) recoverFromBackends() {
	r.mu.RLock()
	empty := r.generation == 0 && len(r.rules) == 0
	r.mu.RUnlock()
	if !empty {
		return
	}
	best := api.RuleSet{}
	for _, backend := range r.o.backends {
		resp, err := (&http.Client{Timeout: r.o.timeout}).Get(backend + "/rules")
		if err != nil {
			continue
		}
		var set api.RuleSet
		decodeErr := json.NewDecoder(resp.Body).Decode(&set)
		_ = resp.Body.Close()
		if decodeErr == nil && set.Generation > best.Generation && len(set.Rules) > 0 {
			best = set
		}
	}
	if best.Generation == 0 || len(best.Rules) == 0 {
		return
	}
	r.mu.Lock()
	r.generation = best.Generation
	r.rules = map[string]api.Rule{}
	for _, rule := range best.Rules {
		if rule.ID != "" {
			r.rules[rule.ID] = rule
		}
	}
	r.mu.Unlock()
	r.save()
	r.log.Info("recovered rules from shards", zap.Int64("generation", best.Generation), zap.Int("rules", len(best.Rules)))
}

// fetchResults fans out to every shard for a rule's materialized results and
// merges them. Sharding is by repo, so a repo appears on one shard; merging is a
// concatenation with a defensive de-dup, ordered by repo (no ranking).
func (r *ruleRegistry) fetchResults(id string) (api.RuleResults, error) {
	rule, ok := r.get(id)
	if !ok {
		return api.RuleResults{}, fmt.Errorf("no such rule: %s", id)
	}
	type shardResult struct {
		res api.RuleResults
		err error
	}
	ch := make(chan shardResult, len(r.o.backends))
	for _, backend := range r.o.backends {
		go func(base string) {
			res, err := r.fetchShardResults(base, id)
			ch <- shardResult{res: res, err: err}
		}(backend)
	}
	merged := map[string]api.RuleRepoResult{}
	truncated := false
	for range r.o.backends {
		sr := <-ch
		if sr.err != nil {
			continue
		}
		truncated = truncated || sr.res.Truncated
		for _, repo := range sr.res.Repos {
			merged[repo.Repo] = repo
		}
	}
	out := api.RuleResults{Rule: id, Mode: rule.Mode, Generation: r.currentGeneration(), Truncated: truncated}
	for _, repo := range merged {
		out.Repos = append(out.Repos, repo)
	}
	search.SortRuleRepoResults(out.Repos)
	if out.Repos == nil {
		out.Repos = []api.RuleRepoResult{}
	}
	return out, nil
}

// fetchShardResults retrieves one shard's materialized result rows for a rule.
// Errors (unreachable shard, undecodable body) are returned for the caller to
// skip — a slow or restarting shard must not fail the whole fetch.
func (r *ruleRegistry) fetchShardResults(base, id string) (api.RuleResults, error) {
	resp, err := (&http.Client{Timeout: r.o.timeout}).Get(base + "/rules/" + id + "/results")
	if err != nil {
		return api.RuleResults{}, err
	}

	defer resp.Body.Close()
	var res api.RuleResults
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return api.RuleResults{}, err
	}
	return res, nil
}

func (r *ruleRegistry) currentGeneration() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.generation
}
