package shard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"grepple/internal/api"
	"grepple/internal/repository"
	"grepple/internal/search"

	"github.com/gofiber/fiber/v3"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"go.uber.org/zap"
)

// shard serves the shard HTTP API and exposes the narrow domain operations
// needed by its HTTP transport and rule evaluator.
type shard interface {
	HealthService
	Service
	RuleService
	Handler() *fiber.App
	Run() error
	indexedHead(string) (string, bool)
}

//grepple:filelocal
type shardImpl struct {
	options       shardOptions
	registry      *repository.Registry
	root          string
	zoektService  zoektService
	repositoryMux sync.RWMutex
	logger        *zap.Logger
	rules         ruleService
}

func (s *shardImpl) runSearchRequest(r api.SearchRequest, reqID string) (api.SearchResponse, error) {
	start := time.Now()
	var t searchTimings
	p, e := search.ResolveRequest(r)
	if e != nil {
		return api.SearchResponse{}, e
	}
	if s != nil {
		p.Root = s.root
	}
	s.lockRepositories(&t)
	defer s.unlockRepositories()
	matchingRepos := s.filterMatchingRepos(p, &t)
	if s != nil && len(p.Repo) > 0 && len(matchingRepos) == 0 {
		return api.SearchResponse{Results: []api.FileResult{}}, nil
	}
	if p.Files {
		return s.runFilesRequest(p, matchingRepos, start, &t, reqID)
	}
	if resp, ok := s.tryZoektCount(p, start, &t, reqID); ok {
		return resp, nil
	}
	zoektStart := time.Now()
	candidates, truncated, scanFallbackRepos, uncoveredRepos := s.gatherCandidates(p, &t, reqID)
	t.zoekt = time.Since(zoektStart).Milliseconds()
	searchStart := time.Now()
	ms, e := search.Files(p, candidates)
	if e != nil {
		return api.SearchResponse{}, e
	}
	t.search = time.Since(searchStart).Milliseconds()
	if p.CountByRepo {
		// Compact per-repo tallies over the match set (no window), so the router
		// can sum a tiny payload instead of shipping every file body.
		t.total = time.Since(start).Milliseconds()
		s.logSearch(reqID, p, t, len(candidates), len(ms), truncated, scanFallbackRepos, uncoveredRepos)
		return api.SearchResponse{RepoCounts: search.AggregateRepoCounts(ms), Truncated: truncated}, nil
	}
	return s.buildResultResponse(p, ms, candidates, truncated, scanFallbackRepos, uncoveredRepos, start, &t, reqID), nil
}

func (s *shardImpl) Handler() *fiber.App {
	app := fiber.New(fiber.Config{
		BodyLimit:      32 << 20,
		ReadBufferSize: 64 << 10,
		ReadTimeout:    30 * time.Second,
		IdleTimeout:    60 * time.Second,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			status := fiber.StatusInternalServerError
			if fiberError, ok := err.(*fiber.Error); ok {
				status = fiberError.Code
			}
			return c.Status(status).JSON(fiber.Map{"error": fmt.Sprint(err)})
		},
	})

	app.Use(recoverer.New())

	var ruleAPI RuleService
	if s.rules != nil {
		ruleAPI = s
	}
	Register(app, s, s, ruleAPI)
	app.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	})
	return app
}

func (s *shardImpl) indexRepo(info api.RepoInfo) error {
	if err := s.zoektService.index(info); err != nil {
		return err
	}
	if info.Head == nil || *info.Head == "" {
		fmt.Fprintf(os.Stderr, "repository %s has no commits; skipping index until it has content\n", info.Repo)
	} else {
		coverage, _ := s.zoektService.coverageFor(info.Repo)
		if !coverage.known {
			fmt.Fprintf(os.Stderr, "could not inspect Zoekt coverage for %s; searches on this shard will use scanner fallback\n", info.Repo)
		} else if len(coverage.fallback) > 0 {
			fmt.Fprintf(os.Stderr, "Zoekt index for %s supplemented by %d scanner-only files\n", info.Repo, len(coverage.fallback))
		}
	}
	if s.rules != nil {
		s.rules.enqueueRepo(info.Repo)
	}
	return nil
}

// primeCoverage optimistically marks every known repository's coverage as known
// at startup so searches use the existing Zoekt shards immediately (repos
// without a shard simply contribute no candidates) instead of forcing a full
// filesystem scan of the whole shard. The background pass replaces these with
// accurate coverage, including scanner-only fallback files.
func (s *shardImpl) primeCoverage() {
	s.zoektService.primeCoverage(s.registry.List())
}

// backgroundReindex reconciles every repository on disk after startup: it
// re-indexes those whose head changed (or whose shard is missing) and computes
// accurate coverage, without blocking readiness. It stops when ctx is done.
func (s *shardImpl) backgroundReindex(ctx context.Context) {
	repos := s.registry.List()
	for _, repo := range repos {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := s.reconcileRepoIndex(repo); err != nil {
			fmt.Fprintf(os.Stderr, "background reindex of %s failed: %v\n", repo.Repo, err)
		}
	}
	fmt.Fprintf(os.Stderr, "background reindex complete (%d repos)\n", len(repos))
}

// reconcileRepoIndex brings a single repository's index up to date. Unchanged
// repositories (matching head in directory.json with an existing shard) only
// have their accurate coverage recomputed; changed, new, or empty repositories
// are (re)indexed via indexRepo.
func (s *shardImpl) reconcileRepoIndex(info api.RepoInfo) error {
	if s.zoektService.upToDate(info) {
		// Already up to date: replace optimistic coverage with the accurate one.
		s.zoektService.setCoverage(info.Repo, inspectZoektCoverage(info.Dir))
		return nil
	}
	return s.indexRepo(info)
}

// logSearch emits one structured info line per search with the request id (set
// by the router so a fan-out can be correlated across shards) and the per-phase
// timings. See docs/logging.md.
func (s *shardImpl) logSearch(reqID string, p search.Params, t searchTimings, candidates, files int, truncated bool, scanFallbackRepos int, uncoveredRepos []string) {
	// api.Rule evaluations reuse the search path but must not emit a per-repo/per-rule
	// search line (the rule worker logs one aggregate line instead); see the rule
	// worker and docs/logging.md.
	if strings.HasPrefix(reqID, "rule:") {
		return
	}
	s.log().Info("search",
		zap.String("reqId", reqID),
		zap.Int64("totalMs", t.total),
		zap.Int64("lockMs", t.lock),
		zap.Int64("filterMs", t.filter),
		zap.Int64("supplementMs", t.supplement),
		zap.Int64("zoektMs", t.zoekt),
		zap.Int64("searchMs", t.search),
		zap.Int64("segmentsMs", t.segments),
		zap.Int("candidates", candidates),
		zap.Int("files", files),
		zap.Int("queryLen", len(p.Query)),
		zap.Bool("countByRepo", p.CountByRepo),
		zap.Int("repoFilter", len(p.Repo)),
		zap.Bool("truncated", truncated),
		// scanFallbackRepos = unknown-coverage repos with no shard that were scanned
		// directly; uncoveredRepos names every unknown-coverage repo (scanned or
		// trusted-via-shard). A persistent entry points at a repo stuck (re)indexing
		// or one whose coverage inspection keeps failing — investigate that repo.
		zap.Int("scanFallbackRepos", scanFallbackRepos),
		zap.Strings("uncoveredRepos", uncoveredRepos),
	)
}

// countRepos returns this shard's repositories that pass the request's --repo /
// --exclude-repo filters (all of them when neither is set). Used to scope
// index-based counts to the requested repositories.
func (s *shardImpl) countRepos(p search.Params) []api.RepoInfo {
	var out []api.RepoInfo
	repoFilter := search.NewRepoFilter(p.Repo, p.ExcludeRepo)
	for _, info := range s.registry.List() {
		if repoFilter.Allow(info.Repo) {
			out = append(out, info)
		}
	}
	return out
}

// log returns the shard's logger, or a no-op logger when unset (e.g. in tests),
// so callers never need a nil check.
func (s *shardImpl) log() *zap.Logger {
	if s == nil || s.logger == nil {
		return zap.NewNop()
	}
	return s.logger
}

// newShard initializes a shard from parsed options.
func newShard(options shardOptions) (shard, error) {
	rootPath, err := filepath.Abs(options.root)
	if err != nil {
		return nil, err
	}
	indexPath, err := filepath.Abs(options.zoekt.indexDir)
	if err != nil {
		return nil, err
	}
	options.root = rootPath
	options.zoekt.indexDir = indexPath
	options.zoekt.repoRoot = rootPath
	if err = os.MkdirAll(options.root, 0755); err != nil {
		return nil, err
	}
	if err = os.Chdir(options.root); err != nil {
		return nil, err
	}
	root, _ := os.Getwd()
	registry := repository.NewRegistry(root)
	if err = registry.Init(); err != nil {
		return nil, err
	}
	logger := newLogger()
	s := &shardImpl{options: options, registry: registry, root: root, logger: logger}
	s.zoektService = newZoektService(options.zoekt, loadDirectory(filepath.Join(root, "directory.json")))
	return s, nil
}

// Run is the shard entry point: it parses args, constructs a shard, and runs it.
// args excludes argv[0].
func Run(args []string) error {
	options, err := parseShardOptions(args)
	if errors.Is(err, errShardHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	s, err := newShard(options)
	if err != nil {
		return err
	}
	return s.Run()
}

// Run serves search over the shard's checkout base until the process is signaled.
func (s *shardImpl) Run() error {
	options := s.options
	defer func() { _ = s.logger.Sync() }()
	if err := os.MkdirAll(options.zoekt.indexDir, 0755); err != nil {
		return err
	}
	if err := reconcileZoektShards(options.zoekt, s.registry.List()); err != nil {
		return fmt.Errorf("reconcile Zoekt shards: %w", err)
	}
	// Fast startup: load the directory of previously-indexed repos (repo -> head)
	// and trust the existing Zoekt shards on disk. Re-indexing of changed/new
	// repos happens asynchronously below, so the shard is ready in seconds and
	// the index becomes eventually consistent in the background.
	s.primeCoverage()
	if err := s.zoektService.start(); err != nil {
		return fmt.Errorf("start Zoekt: %w", err)
	}
	defer s.zoektService.stop()
	fmt.Fprintf(os.Stderr, "zoekt starting in background (index: %s)\n", options.zoekt.indexDir)
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", options.host, options.port))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "grepple shard listening on http://%s:%d (root: %s, repos: %d)\n", options.host, listener.Addr().(*net.TCPAddr).Port, s.root, len(s.registry.List()))
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Reconcile the index in the background and evaluate predefined grep rules.
	s.rules = newRuleService(filepath.Join(s.root, "rules.json"), s, s.logger)
	go s.rules.run(shutdown)
	go s.backgroundReindex(shutdown)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- s.Handler().Listener(listener, fiber.ListenConfig{
			DisableStartupMessage: true,
			GracefulContext:       shutdown,
			ShutdownTimeout:       10 * time.Second,
		})
	}()
	select {
	case err = <-serverDone:
		s.zoektService.stop()
		return err
	case <-s.zoektService.done():
		if shutdown.Err() != nil {
			return <-serverDone
		}
		zoektErr := s.zoektService.err()
		stop()
		<-serverDone
		return fmt.Errorf("zoekt-webserver stopped unexpectedly: %v", zoektErr)
	}
}

func (s *shardImpl) Root() string { return s.root }

func (s *shardImpl) ZoektRunning() bool {
	return s.zoektService != nil && s.zoektService.running()
}

func (s *shardImpl) Search(request api.SearchRequest, requestID string) (api.SearchResponse, error) {
	return s.runSearchRequest(request, requestID)
}

func (s *shardImpl) ListRepos() []api.RepoInfo { return s.registry.List() }

func (s *shardImpl) GetRepo(repo string) (api.RepoInfo, bool) {
	return s.registry.Get(repo)
}

func (s *shardImpl) indexedHead(repo string) (string, bool) {
	if s.zoektService == nil {
		return "", false
	}
	return s.zoektService.indexedHead(repo)
}

func (s *shardImpl) CreateRepo(ref api.RepoRef, token string) (api.RepoInfo, error) {
	s.repositoryMux.Lock()
	defer s.repositoryMux.Unlock()
	info, err := s.registry.Add(ref.Repo, ref.URL, ref.Ref, token)
	if err != nil {
		return info, err
	}
	if err := s.indexRepo(info); err != nil {
		return info, StatusError{Status: 500, Err: fmt.Errorf("index %s with Zoekt: %w", info.Repo, err)}
	}
	return info, nil
}

func (s *shardImpl) RefreshRepo(ref api.RepoRef, token string) (api.RepoInfo, error) {
	s.repositoryMux.Lock()
	defer s.repositoryMux.Unlock()
	info, err := s.registry.Pull(ref.Repo, ref.URL, ref.Ref, token)
	if err != nil {
		return info, err
	}
	if err := s.indexRepo(info); err != nil {
		return info, StatusError{Status: 500, Err: fmt.Errorf("index %s with Zoekt: %w", info.Repo, err)}
	}
	return info, nil
}

func (s *shardImpl) RemoveRepo(repo string) (bool, error) {
	s.repositoryMux.Lock()
	defer s.repositoryMux.Unlock()
	removed, err := s.registry.Remove(repo)
	if err != nil || !removed {
		return removed, err
	}
	err = s.zoektService.remove(repo)
	if s.rules != nil {
		s.rules.forgetRepo(repo)
	}
	if err != nil {
		return removed, StatusError{Status: 500, Err: fmt.Errorf("remove %s from Zoekt: %w", repo, err)}
	}
	return removed, nil
}

func (s *shardImpl) ApplyRules(set api.RuleSet) bool { return s.rules.apply(set) }
func (s *shardImpl) RulesGeneration() int64          { return s.rules.currentGeneration() }
func (s *shardImpl) RulesSnapshot() []api.Rule       { return s.rules.snapshotRules() }
func (s *shardImpl) RuleResults(id string) (string, []api.RuleRepoResult) {
	return s.rules.resultsFor(id)
}

// searchTimings holds the per-phase wall-clock durations (milliseconds) for one
// search, so a slow shard shows exactly which step is expensive: waiting on the
// repository lock, the repo pre-filter, priming Zoekt fallback coverage (holds
// the index mutex — contends with a background reindex), the Zoekt candidate
// fetch, the file content search, or tree-sitter segment building.
type searchTimings struct {
	total, lock, filter, supplement, zoekt, search, segments int64
}

// lockRepositories takes the registry read lock and records the wait time.
func (s *shardImpl) lockRepositories(t *searchTimings) {
	if s == nil {
		return
	}
	lockStart := time.Now()
	s.repositoryMux.RLock()
	t.lock = time.Since(lockStart).Milliseconds()
}

func (s *shardImpl) unlockRepositories() {
	if s != nil {
		s.repositoryMux.RUnlock()
	}
}

// filterMatchingRepos resolves the request's repo scope to the repositories
// this shard actually hosts (using the same predicates the search engine
// applies per file), so an unmatched shard can return immediately — no Zoekt
// query, no filesystem walk. Nil when no state or no repo filter applies.
func (s *shardImpl) filterMatchingRepos(p search.Params, t *searchTimings) []string {
	if s == nil || len(p.Repo) == 0 {
		return nil
	}
	filterStart := time.Now()
	repoFilter := search.NewRepoFilter(p.Repo, p.ExcludeRepo)
	var matchingRepos []string
	for _, info := range s.registry.List() {
		if repoFilter.Allow(info.Repo) {
			matchingRepos = append(matchingRepos, info.Repo)
		}
	}
	t.filter = time.Since(filterStart).Milliseconds()
	return matchingRepos
}

// runFilesRequest serves a file-listing (no content) request: the listing is
// scoped to the matching repositories' directories when a repo filter is set,
// so only those trees are walked rather than the whole shard.
func (s *shardImpl) runFilesRequest(p search.Params, matchingRepos []string, start time.Time, t *searchTimings, reqID string) (api.SearchResponse, error) {
	listStart := time.Now()
	var (
		paths []string
		e     error
	)
	if s != nil && len(matchingRepos) > 0 {
		dirs := make([]string, 0, len(matchingRepos))
		for _, repo := range matchingRepos {
			if info, ok := s.registry.Get(repo); ok {
				dirs = append(dirs, info.Dir)
			}
		}
		var files []string
		if files, e = search.CollectFilesUnder(dirs, s.root); e == nil {
			paths, e = search.ListFilePaths(p, files)
		}
	} else {
		paths, e = search.ListFilePaths(p, nil)
	}
	if e != nil {
		return api.SearchResponse{}, e
	}
	rs := make([]api.FileResult, 0, len(paths))
	for _, x := range paths {
		fr := api.FileResult{Path: x, Language: "", Matches: []api.ResultMatch{}, Segments: []api.ResultSegment{}}
		s.annotateResultRepo(&fr)
		rs = append(rs, fr)
	}
	t.search = time.Since(listStart).Milliseconds()
	t.total = time.Since(start).Milliseconds()
	s.logSearch(reqID, p, *t, 0, len(rs), false, 0, nil)
	return api.SearchResponse{Results: rs}, nil
}

// tryZoektCount answers a count-by-repo request entirely from the Zoekt index
// (no filesystem reads), reporting ok=false when the shard must fall through
// to the scan-based count (no index, query too short, or an index error).
func (s *shardImpl) tryZoektCount(p search.Params, start time.Time, t *searchTimings, reqID string) (api.SearchResponse, bool) {
	if !p.CountByRepo || s == nil || !s.ZoektRunning() {
		return api.SearchResponse{}, false
	}
	q := zoektQuery(p)
	if q == "" {
		return api.SearchResponse{}, false
	}
	countStart := time.Now()
	counts, ztrunc, err := s.zoektService.repoCounts(q, s.countRepos(p), p.Globs)
	t.zoekt = time.Since(countStart).Milliseconds()
	if err != nil {
		s.log().Warn("zoekt count failed, falling back to scan", zap.String("reqId", reqID), zap.Error(err))
		return api.SearchResponse{}, false
	}
	totalFiles := 0
	for _, c := range counts {
		totalFiles += c.Files
	}
	t.total = time.Since(start).Milliseconds()
	s.logSearch(reqID, p, *t, 0, totalFiles, ztrunc, 0, nil)
	return api.SearchResponse{RepoCounts: counts, Truncated: ztrunc}, true
}

// gatherCandidates builds the scan candidate list from the Zoekt index plus
// the supplemental fallback files; nil candidates mean a full scan. Also
// reports whether the index truncated and which repos fell back to scanning.
func (s *shardImpl) gatherCandidates(p search.Params, t *searchTimings, reqID string) (candidates []string, truncated bool, scanFallbackRepos int, uncoveredRepos []string) {
	if s == nil || !s.ZoektRunning() {
		return nil, false, 0, nil
	}
	q := zoektQuery(p)
	if q == "" {
		return nil, false, 0, nil
	}
	// NOTE: the matching repos are NOT pushed into the Zoekt query as repo:
	// atoms — Files post-filters by repo, and a repo: clause proved fragile in
	// Zoekt's query grammar for names containing '-'. The empty-shard fast path
	// already skips shards with no matching repo.
	supplementStart := time.Now()
	supplement, scanDirs, uncov := s.zoektSupplementalCandidates()
	t.supplement = time.Since(supplementStart).Milliseconds()
	xs, zoektTruncated, err := s.zoektService.candidates(q, s.root, s.registry.List())
	if err != nil {
		// A real Zoekt failure (not truncation): fall back to a full scan.
		s.log().Warn("zoekt search failed, falling back to full scan", zap.String("reqId", reqID), zap.Error(err))
		return nil, false, len(scanDirs), uncov
	}
	set := map[string]bool{}
	for _, path := range append(xs, supplement...) {
		set[path] = true
	}
	// Per-repo fallback: scan only the directories of unknown-coverage repos that
	// have no Zoekt shard at all (a covered/indexed repo is served from the
	// index). This keeps one lagging repo from forcing a full scan.
	if len(scanDirs) > 0 {
		if scanned, e := search.CollectFilesUnder(scanDirs, s.root); e == nil {
			for _, path := range scanned {
				set[path] = true
			}
		}
	}
	return sortedPaths(set), zoektTruncated, len(scanDirs), uncov
}

// buildResultResponse renders the match set into the wire response: structural
// segments (unless skipped), repo attribution per file, and the search log.
func (s *shardImpl) buildResultResponse(p search.Params, ms []search.FileMatch, candidates []string, truncated bool, scanFallbackRepos int, uncoveredRepos []string, start time.Time, t *searchTimings, reqID string) api.SearchResponse {
	segmentsStart := time.Now()
	rs := search.BuildResults(ms, p.BeforeContext, p.AfterContext, p.MaxSegments, !p.SkipSegments)
	for index := range rs {
		s.annotateResultRepo(&rs[index])
	}
	t.segments = time.Since(segmentsStart).Milliseconds()
	t.total = time.Since(start).Milliseconds()
	s.logSearch(reqID, p, *t, len(candidates), len(ms), truncated, scanFallbackRepos, uncoveredRepos)
	return api.SearchResponse{Results: rs, Truncated: truncated}
}

// zoektSupplementalCandidates returns, under the index lock, the scanner-only
// fallback files for repositories whose Zoekt coverage is known, the directories
// of repositories that must be scanned directly, and the names of all repos
// whose coverage is unknown (for diagnostics).
//
// Coverage is per-repo, and an unknown-coverage repo is only scanned when it has
// NO Zoekt shard at all. If it already has a shard (it was indexed; only the
// coverage inspection failed) we trust the index for it rather than scanning the
// whole repo — we merely lose its untracked/oversized fallback files. This keeps
// one repo whose inspection keeps failing from dragging tens of thousands of
// files into every search.
func (s *shardImpl) zoektSupplementalCandidates() (fallback []string, scanDirs []string, uncoveredRepos []string) {
	for _, repo := range s.registry.List() {
		coverage, ok := s.zoektService.coverageFor(repo.Repo)
		if ok && coverage.known {
			fallback = append(fallback, coverage.fallback...)
			continue
		}
		uncoveredRepos = append(uncoveredRepos, repo.Repo)
		// Unknown coverage: trust an existing shard, only scan when there is none.
		if !s.zoektService.shardExists(repo.Repo) {
			scanDirs = append(scanDirs, repo.Dir)
		}
	}
	return fallback, scanDirs, uncoveredRepos
}

// annotateResultRepo stamps the owning repository ID on a result when the
// shard's registry knows it; results for unknown paths stay unscoped.
func (s *shardImpl) annotateResultRepo(fr *api.FileResult) {
	if s == nil {
		return
	}
	repo := search.RepoID(fr.Path)
	if _, ok := s.registry.Get(repo); ok {
		fr.Repo = repo
	}
}
