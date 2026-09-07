package shard

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"grepple/internal/api"
	"grepple/internal/search"

	"go.uber.org/zap"
)

// maxRulePaths caps how many matching file paths a files-mode rule stores per
// repository, so one giant match set can't blow up shard memory. The result is
// still useful (files/matches counts are exact); only the path list is capped.
const maxRulePaths = 1000

// ruleEvalQueue bounds the pending evaluation backlog. Reindexes enqueue a repo
// for re-evaluation; if the shard falls far behind, the periodic reconcile and
// the router's periodic rule re-push both re-trigger evaluation, so a dropped
// enqueue is recovered rather than lost forever.
const ruleEvalQueue = 4096

// evalJob is one unit of rule evaluation work. Exactly one of repo/ruleID is set:
//   - repo set: re-evaluate every rule for that one repository (after a reindex).
//   - ruleID set: backfill that one rule across every repository (after a rule
//     is created or changed).
type evalJob struct {
	repo   string
	ruleID string
}

// ruleServiceImpl holds the shard's copy of the rule definitions (pushed by the
// router) and the materialized per-repo results, and drives their incremental
// re-evaluation. Definitions and results are persisted to rules.json so a
// restart can serve the last-known results immediately.
//
//grepple:filelocal
type ruleServiceImpl struct {
	mu         sync.RWMutex
	generation int64
	rules      map[string]api.Rule
	results    map[string]map[string]api.RuleRepoResult // ruleID -> repo -> result

	s      shard
	path   string
	evalCh chan evalJob
	log    *zap.Logger
}

type ruleStateFile struct {
	Generation int64                                    `json:"generation"`
	Rules      []api.Rule                               `json:"rules"`
	Results    map[string]map[string]api.RuleRepoResult `json:"results"`
}

type ruleService interface {
	apply(api.RuleSet) bool
	snapshotRules() []api.Rule
	currentGeneration() int64
	resultsFor(string) (string, []api.RuleRepoResult)
	enqueueRepo(string)
	forgetRepo(string)
	run(context.Context)
}

func newRuleService(path string, s shard, logger *zap.Logger) ruleService {
	if logger == nil {
		logger = zap.NewNop()
	}
	rs := &ruleServiceImpl{
		rules:   map[string]api.Rule{},
		results: map[string]map[string]api.RuleRepoResult{},
		s:       s,
		path:    path,
		evalCh:  make(chan evalJob, ruleEvalQueue),
		log:     logger,
	}
	rs.load()
	return rs
}

func (rs *ruleServiceImpl) load() {
	data, err := os.ReadFile(rs.path)
	if err != nil {
		return
	}
	var file ruleStateFile
	if json.Unmarshal(data, &file) != nil {
		return
	}
	rs.generation = file.Generation
	for _, r := range file.Rules {
		if r.ID != "" {
			rs.rules[r.ID] = r
		}
	}
	if file.Results != nil {
		rs.results = file.Results
	}
}

// save persists the current definitions and results. Callers must not hold rs.mu
// exclusively in a way that conflicts; save takes a read lock itself.
func (rs *ruleServiceImpl) save() {
	rs.mu.RLock()
	file := ruleStateFile{Generation: rs.generation, Results: rs.results}
	for _, r := range rs.rules {
		file.Rules = append(file.Rules, r)
	}
	rs.mu.RUnlock()
	sort.Slice(file.Rules, func(i, j int) bool { return file.Rules[i].ID < file.Rules[j].ID })
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return
	}
	tmp := rs.path + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, rs.path)
	}
}

// apply installs a ruleset pushed by the router. Older or equal generations are
// ignored (idempotent, tolerant of re-pushes). It drops results for removed
// rules and enqueues a backfill for every new or changed rule. Returns whether
// the set was applied.
func (rs *ruleServiceImpl) apply(set api.RuleSet) bool {
	rs.mu.Lock()
	if set.Generation != 0 && set.Generation < rs.generation {
		rs.mu.Unlock()
		return false
	}
	next := map[string]api.Rule{}
	for _, r := range set.Rules {
		if r.ID != "" {
			next[r.ID] = r
		}
	}
	var changed []string
	for id, r := range next {
		if old, ok := rs.rules[id]; !ok || !sameRule(old, r) {
			changed = append(changed, id)
		}
	}
	for id := range rs.rules {
		if _, ok := next[id]; !ok {
			delete(rs.results, id) // rule removed: drop its materialized results
		}
	}
	rs.rules = next
	rs.generation = set.Generation
	rs.mu.Unlock()
	rs.save()
	for _, id := range changed {
		rs.enqueue(evalJob{ruleID: id})
	}
	return true
}

func sameRule(a, b api.Rule) bool {
	x, _ := json.Marshal(a.Request)
	y, _ := json.Marshal(b.Request)
	return a.Mode == b.Mode && string(x) == string(y)
}

func (rs *ruleServiceImpl) snapshotRules() []api.Rule {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	out := make([]api.Rule, 0, len(rs.rules))
	for _, r := range rs.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (rs *ruleServiceImpl) currentGeneration() int64 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.generation
}

// resultsFor returns this shard's materialized results for a rule, sorted by
// repository.
func (rs *ruleServiceImpl) resultsFor(ruleID string) (string, []api.RuleRepoResult) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	mode := ""
	if r, ok := rs.rules[ruleID]; ok {
		mode = r.Mode
	}
	byRepo := rs.results[ruleID]
	out := make([]api.RuleRepoResult, 0, len(byRepo))
	for _, r := range byRepo {
		out = append(out, r)
	}
	search.SortRuleRepoResults(out)
	return mode, out
}

// enqueue submits a job without blocking the caller (a reindex must not stall on
// rule evaluation). A full queue is logged and dropped; periodic re-index and
// rule re-push recover it.
func (rs *ruleServiceImpl) enqueue(job evalJob) {
	select {
	case rs.evalCh <- job:
	default:
		rs.log.Warn("rule eval queue full; dropping job", zap.String("repo", job.repo), zap.String("rule", job.ruleID))
	}
}

func (rs *ruleServiceImpl) enqueueRepo(repo string) {
	if repo != "" {
		rs.enqueue(evalJob{repo: repo})
	}
}

// forgetRepo drops a repository from every rule's results (called when the repo
// is removed from this shard).
func (rs *ruleServiceImpl) forgetRepo(repo string) {
	rs.mu.Lock()
	for _, byRepo := range rs.results {
		delete(byRepo, repo)
	}
	rs.mu.Unlock()
	rs.save()
}

// run drains the evaluation queue on a single goroutine (serialized so rule
// evaluation never overloads the shard) until the context is cancelled.
func (rs *ruleServiceImpl) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-rs.evalCh:
			if job.repo != "" {
				rs.evaluateRepo(job.repo)
			} else if job.ruleID != "" {
				rs.backfillRule(job.ruleID)
			}
		}
	}
}

// evaluateRepo re-evaluates every rule against a single repository and updates
// that repo's row in each rule's results.
func (rs *ruleServiceImpl) evaluateRepo(repo string) {
	rules := rs.snapshotRules()
	if len(rules) == 0 {
		return
	}
	start := time.Now()
	for _, rule := range rules {
		p, err := search.ResolveRequest(rule.Request)
		if err != nil {
			continue
		}
		// Honor the rule's own repo filter: a repo it doesn't target has no row.
		if !search.NewRepoFilter(p.Repo, p.ExcludeRepo).Allow(repo) {
			rs.setResult(rule.ID, repo, api.RuleRepoResult{}, false)
			continue
		}
		req := rule.Request
		req.Repo = []string{repo}
		res := rs.evaluate(rule, req)
		got, ok := res[repo]
		rs.setResult(rule.ID, repo, got, ok)
	}
	rs.save()
	rs.log.Info("rules evaluated",
		zap.String("repo", repo),
		zap.Int("rules", len(rules)),
		zap.Int64("ms", time.Since(start).Milliseconds()),
	)
}

// backfillRule evaluates one rule across every repository on this shard in a
// single pass and replaces that rule's results.
func (rs *ruleServiceImpl) backfillRule(ruleID string) {
	rs.mu.RLock()
	rule, ok := rs.rules[ruleID]
	rs.mu.RUnlock()
	if !ok {
		return
	}
	start := time.Now()
	res := rs.evaluate(rule, rule.Request)
	rs.mu.Lock()
	rs.results[ruleID] = res
	rs.mu.Unlock()
	rs.save()
	rs.log.Info("rule backfilled",
		zap.String("rule", ruleID),
		zap.Int("repos", len(res)),
		zap.Int64("ms", time.Since(start).Milliseconds()),
	)
}

// evaluate runs the rule's search (scoped as given in req) and converts the
// response into per-repo results keyed by repository.
func (rs *ruleServiceImpl) evaluate(rule api.Rule, req api.SearchRequest) map[string]api.RuleRepoResult {
	switch rule.Mode {
	case api.RuleModeFiles:
		req.Files = true
		req.CountByRepo = false
	default:
		req.CountByRepo = true
		req.Files = false
	}
	resp, err := rs.s.Search(req, "rule:"+rule.ID)
	if err != nil {
		rs.log.Warn("rule evaluation failed", zap.String("rule", rule.ID), zap.Error(err))
		return map[string]api.RuleRepoResult{}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := map[string]api.RuleRepoResult{}
	if rule.Mode == api.RuleModeFiles {
		accumulateFilesResults(out, resp.Results)
	} else {
		for _, c := range resp.RepoCounts {
			out[c.Repo] = api.RuleRepoResult{Repo: c.Repo, Files: c.Files, Matches: c.Matches}
		}
	}
	for repo, r := range out {
		// Only registered repositories count: the scan fallback can otherwise pick
		// up shard-local files (rules.json, directory.json) that are not part of any
		// repo. The Zoekt path is already git-tracked-only, so this is belt-and-braces.
		if _, ok := rs.s.GetRepo(repo); !ok {
			delete(out, repo)
			continue
		}
		if head, ok := rs.s.indexedHead(repo); ok {
			r.Head = head
		}
		r.EvaluatedAt = now
		sort.Strings(r.Paths)
		out[repo] = r
	}
	return out
}

// accumulateFilesResults folds a files-mode search response into per-repo
// results, deriving the repo from the path when the shard left it unset and
// capping stored paths at maxRulePaths (counts stay exact regardless).
func accumulateFilesResults(out map[string]api.RuleRepoResult, results []api.FileResult) {
	for _, fr := range results {
		repo := fr.Repo
		if repo == "" {
			repo = search.RepoID(fr.Path)
		}
		r := out[repo]
		r.Repo = repo
		r.Files++
		if len(r.Paths) < maxRulePaths {
			r.Paths = append(r.Paths, fr.Path)
		}
		out[repo] = r
	}
}

// setResult stores or clears a single repo's row for a rule. present=false (or a
// zero result) removes the row so a repo that no longer matches disappears.
func (rs *ruleServiceImpl) setResult(ruleID, repo string, r api.RuleRepoResult, present bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if !present || (r.Files == 0 && r.Matches == 0 && len(r.Paths) == 0) {
		if byRepo := rs.results[ruleID]; byRepo != nil {
			delete(byRepo, repo)
		}
		return
	}
	if rs.results[ruleID] == nil {
		rs.results[ruleID] = map[string]api.RuleRepoResult{}
	}
	rs.results[ruleID][repo] = r
}
