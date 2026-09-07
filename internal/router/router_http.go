package router

import (
	"context"
	"encoding/json"
	"fmt"
	"grepple/internal/api"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	proxyMiddleware "github.com/gofiber/fiber/v3/middleware/proxy"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"go.uber.org/zap"
)

type routerState struct {
	o         routerOptions
	q         *jobQueue
	proxies   map[string]fiber.Handler
	logger    *zap.Logger
	authCache *tokenAuthCache
	rules     *ruleRegistry
}

func checkBackendHealth(o routerOptions) ([]fiber.Map, bool) {
	type result struct {
		backend string
		status  int
		err     error
	}
	results := make(chan result, len(o.backends))
	client := http.Client{Timeout: min(o.timeout, 2*time.Second)}
	for _, backend := range o.backends {
		go func(base string) {
			response, err := client.Get(strings.TrimRight(base, "/") + "/health")
			if err != nil {
				results <- result{backend: base, err: err}
				return
			}
			response.Body.Close()
			results <- result{backend: base, status: response.StatusCode}
		}(backend)
	}
	byBackend := map[string]result{}
	healthy := true
	for range o.backends {
		result := <-results
		byBackend[result.backend] = result
		if result.err != nil || result.status < 200 || result.status >= 300 {
			healthy = false
		}
	}
	statuses := make([]fiber.Map, 0, len(o.backends))
	for _, backend := range o.backends {
		result := byBackend[backend]
		status := fiber.Map{"url": backend, "ok": result.err == nil && result.status >= 200 && result.status < 300}
		if result.status != 0 {
			status["status"] = result.status
		}
		if result.err != nil {
			status["error"] = result.err.Error()
		}
		statuses = append(statuses, status)
	}
	return statuses, healthy
}

func (s *routerState) proxy(c fiber.Ctx, path string) error {
	repo := c.Query("repo", c.Query("repository"))
	if repo == "" {
		return errorJSON(c, fiber.StatusBadRequest, path+" requires 'repo'")
	}
	// Normalize the outbound path to the shard route, stripping any /public
	// prefix the CLI used so the shard receives the path it actually serves.
	c.Request().URI().SetPath(path)
	handler := s.proxies[s.o.ring.get(repo)]
	if handler == nil {
		return errorJSON(c, fiber.StatusBadGateway, "backend is not configured")
	}
	if err := handler(c); err != nil {
		return errorJSON(c, fiber.StatusBadGateway, err)
	}
	return nil
}

func (s *routerState) handleIndex(c fiber.Ctx) error {
	repo := c.Query("repo", c.Query("repository"))
	if c.Method() == fiber.MethodGet && repo == "" {
		return writeJSON(c, fiber.StatusOK, fiber.Map{"repos": listIndexed(s.o)})
	}
	ref, err := readRepoRef(c)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	switch c.Method() {
	case fiber.MethodGet, fiber.MethodPost, fiber.MethodPut, fiber.MethodDelete:
		shard := s.o.ring.get(ref.Repo)
		status, result := indexShard(s.o, shard, c.Method(), ref)
		result["shard"] = shard
		return writeJSON(c, status, result)
	default:
		return errorJSON(c, fiber.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *routerState) handler() *fiber.App {
	if s.authCache == nil {
		s.authCache = newTokenAuthCache()
	}
	s.buildProxies()

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
			return errorJSON(c, status, err)
		},
	})
	app.Use(recoverer.New())

	// Liveness: cheap, dependency-free "process is up" check. It must not touch
	// the job queue or backends, so a busy queue or a down shard never restarts
	// the router pod.
	app.Get("/livez", s.handleLivez)
	// Readiness reports only the router's own state and never probes the shards;
	// shard health is available (informationally) on /backends, job stats on /jobs.
	app.Get("/health", s.handleHealth)
	// The client ID is public (not a secret) and lets `grepple login` run the
	// device flow without per-user configuration.
	app.Get("/auth/config", s.handleAuthConfig)
	// Refresh runs server-side because it needs the client secret; it stays open
	// (the caller's access token may be expired — the refresh token is the
	// credential, and GitHub validates it).
	app.Post("/auth/refresh", s.handleAuthRefresh)
	app.Get("/backends", s.handleBackends)
	app.Get("/", s.handleOverview)
	app.Get("/jobs", s.handleJobs)
	// CLI-facing endpoints live under /public and are guarded by requireOrgAuth;
	// /auth/config stays open so the CLI can bootstrap login before it has a token.
	public := app.Group("/public", s.requireOrgAuth())
	public.Get("/raw", s.proxyRaw)
	public.Get("/tree", s.proxyTree)
	public.Get("/repos", s.handleRepos)
	public.Post("/search", s.handlePublicSearch)
	// Predefined searches (rules): definitions live here (source of truth), results
	// are materialized on the shards and aggregated on read.
	public.Post("/rules", s.handleRuleCreate)
	public.Get("/rules", s.handleRuleList)
	public.Get("/rules/:id", s.handleRuleGet)
	public.Delete("/rules/:id", s.handleRuleDelete)
	public.Get("/rules/:id/results", s.handleRuleResults)
	app.All("/index", s.handleIndex)
	app.Post("/github/webhook", s.webhook)
	app.Post("/rebalance", s.handleRebalance)
	app.Post("/reconcile", s.handleReconcile)
	app.Use(s.handleNotFound)
	return app
}

// buildProxies creates one path-prefix-preserving balancer proxy per backend.
// Shard backends are explicitly configured internal services, so private IPs
// are allowed.
func (s *routerState) buildProxies() {
	s.proxies = make(map[string]fiber.Handler, len(s.o.backends))
	policy := proxyMiddleware.DefaultSecurityPolicy()
	policy.AllowPrivateIPs = true
	for _, backend := range s.o.backends {
		upstream, _ := url.Parse(backend)
		basePath := strings.TrimRight(upstream.Path, "/")
		s.proxies[backend] = proxyMiddleware.Balancer(proxyMiddleware.Config{
			Servers:             []string{backend},
			Timeout:             s.o.timeout,
			MaxResponseBodySize: 64 << 20,
			SecurityPolicy:      &policy,
			ModifyRequest: func(c fiber.Ctx) error {
				requestPath := string(c.Request().URI().Path())
				c.Request().URI().SetPath(basePath + requestPath)
				return nil
			},
		})
	}
}

// log returns the router's logger, or a no-op logger when unset (e.g. in tests),
// so callers never need a nil check.
func (s *routerState) log() *zap.Logger {
	if s == nil || s.logger == nil {
		return zap.NewNop()
	}
	return s.logger
}

// requireOrgAuth is the middleware guarding the CLI-facing /public endpoints.
// When auth is not required it is a pass-through (local/dev). Otherwise it
// requires a valid GitHub token (from `grepple login`) whose user is a member of
// an authorized organization. Positively-validated tokens are cached.
func (s *routerState) requireOrgAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		return checkOrgAuth(s, c)
	}
}

// handleLivez answers liveness: the process is up (no dependency checks).
func (s *routerState) handleLivez(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, fiber.Map{"ok": true, "mode": "router"})
}

// handleHealth answers readiness with the router's own state only (watch
// config, token presence, App installations) — never shard health.
func (s *routerState) handleHealth(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, fiber.Map{
		"ok": true, "mode": "router",
		"watch": fiber.Map{"repos": s.o.watchRepos, "orgs": s.o.watchOrgs, "tokenConfigured": s.o.githubToken != "", "githubAppInstallations": len(s.o.auth.installations())},
	})
}

// handleAuthConfig advertises the GitHub OAuth/App client ID and scopes so
// `grepple login` can run the device flow.
func (s *routerState) handleAuthConfig(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, fiber.Map{
		"clientId": s.o.githubClientID,
		"scopes":   s.o.githubScopes,
		"refresh":  s.o.githubClientSecret != "",
	})
}

// handleAuthRefresh exchanges a GitHub App user refresh token for a fresh
// access token so the CLI can renew silently instead of forcing `grepple login`.
func (s *routerState) handleAuthRefresh(c fiber.Ctx) error {
	if s.o.githubClientID == "" || s.o.githubClientSecret == "" {
		return errorJSON(c, fiber.StatusNotImplemented, "token refresh is not configured on this server")
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil || strings.TrimSpace(body.RefreshToken) == "" {
		return errorJSON(c, fiber.StatusBadRequest, "missing refresh_token")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	tok, status, err := refreshUserToken(client, s.o.githubWebBase, s.o.githubClientID, s.o.githubClientSecret, body.RefreshToken)
	if err != nil {
		return errorJSON(c, status, err)
	}
	return writeJSON(c, status, tok)
}

// handleBackends reports each backend's /health and an aggregate ok (503 when
// a shard is unreachable) — informational, never used by probes.
func (s *routerState) handleBackends(c fiber.Ctx) error {
	backendStatus, healthy := checkBackendHealth(s.o)
	status := fiber.StatusOK
	if !healthy {
		status = fiber.StatusServiceUnavailable
	}
	return writeJSON(c, status, fiber.Map{"ok": healthy, "mode": "router", "backends": backendStatus})
}

// handleOverview reports the router's view: the backends plus each shard's
// repositories and total count.
func (s *routerState) handleOverview(c fiber.Ctx) error {
	indexed := listIndexed(s.o)
	shards := make([]fiber.Map, 0, len(s.o.backends))
	for _, backend := range s.o.backends {
		var repos []string
		for _, info := range indexed {
			if info.Shard == backend {
				repos = append(repos, info.Repo)
			}
		}
		sort.Strings(repos)
		shards = append(shards, fiber.Map{"backend": backend, "count": len(repos), "repos": repos})
	}
	return writeJSON(c, fiber.StatusOK, fiber.Map{
		"ok": true, "mode": "router", "backends": s.o.backends,
		"totalRepos": len(indexed), "shards": shards,
	})
}

// handleJobs reports the job-queue stats (kept off /, /health, and probes).
func (s *routerState) handleJobs(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, s.q.stats())
}

// proxyRaw forwards /public/raw to the owning shard's /raw.
func (s *routerState) proxyRaw(c fiber.Ctx) error { return s.proxy(c, "/raw") }

// proxyTree forwards /public/tree to the owning shard's /tree.
func (s *routerState) proxyTree(c fiber.Ctx) error { return s.proxy(c, "/tree") }

// handlePublicSearch fans a CLI search out to the shards and merges the
// response.
func (s *routerState) handlePublicSearch(c fiber.Ctx) error {
	var request api.SearchRequest
	if err := json.Unmarshal(c.Body(), &request); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	response, err := routeSearch(s.o, request)
	if err != nil {
		return errorJSON(c, fiber.StatusBadGateway, err)
	}
	return writeJSON(c, fiber.StatusOK, response)
}

// handleRebalance runs a shard rebalance (?dryRun=1 only plans).
func (s *routerState) handleRebalance(c fiber.Ctx) error {
	dryRun := c.Query("dryRun")
	return writeJSON(c, fiber.StatusOK, rebalance(s.o, dryRun == "1" || dryRun == "true"))
}

func (s *routerState) webhook(c fiber.Ctx) error {
	raw := c.Body()
	if s.o.webhookSecret != "" && !verifyWebhook(s.o.webhookSecret, raw, c.Get("x-hub-signature-256")) {
		return errorJSON(c, fiber.StatusUnauthorized, "invalid webhook signature")
	}
	event := c.Get("x-github-event")
	if event == "ping" {
		return writeJSON(c, fiber.StatusOK, fiber.Map{"ok": true, "pong": true})
	}
	var payload struct {
		Ref        string `json:"ref"`
		Deleted    bool   `json:"deleted"`
		Action     string `json:"action"`
		Repository struct {
			FullName      string `json:"full_name"`
			CloneURL      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	method, action := "", "ignored"
	ref := api.RepoRef{Repo: payload.Repository.FullName, URL: payload.Repository.CloneURL}
	switch {
	case payload.Repository.FullName == "":
		action = "ignored (no repository)"
	case event == "push" && !payload.Deleted:
		method = fiber.MethodPut
		action = "pull"
		ref.Ref = strings.TrimPrefix(payload.Ref, "refs/heads/")
		if ref.Ref == "" {
			ref.Ref = payload.Repository.DefaultBranch
		}
	case event == "push":
		action = "ignored (branch deleted)"
	case event == "repository" && payload.Action == "created":
		method = fiber.MethodPost
		action = "clone"
	case event == "repository" && payload.Action == "deleted":
		method = fiber.MethodDelete
		action = "remove"
	case event == "repository" && payload.Action == "archived":
		// Archiving makes a repo read-only; drop it from the index like a delete
		// so archived repos don't linger (reconcile also prunes them as a backstop).
		method = fiber.MethodDelete
		action = "remove (archived)"
	case event == "repository" && payload.Action == "unarchived":
		method = fiber.MethodPost
		action = "clone (unarchived)"
		ref.Ref = payload.Repository.DefaultBranch
	}
	if method == "" {
		return writeJSON(c, fiber.StatusOK, fiber.Map{"event": event, "repo": payload.Repository.FullName, "action": action})
	}
	id := s.q.enqueue(event, method, ref)
	return writeJSON(c, fiber.StatusAccepted, fiber.Map{
		"event": event, "repo": ref.Repo, "action": action, "jobId": id,
		"shard": s.o.ring.get(ref.Repo), "status": "queued",
	})
}

func (s *routerState) reconcile() map[string]any {
	desired := map[string]api.RepoRef{}
	// pushedAt carries each discovered repo's last-push timestamp (free in the
	// discovery payloads) so the cycle can also refresh stale content, not just
	// the repo set. Allowlist-only repos never get one and rely on webhooks.
	pushedAt := map[string]time.Time{}
	stats := discoveryStats{}
	allowed := repoAllowlist(s.o.watchRepos, desired)
	errorsOut := discoverFromOrgs(s.o.watchOrgs, s.o.githubToken, allowed, desired, pushedAt, &stats)
	errorsOut = append(errorsOut, discoverFromInstallations(s.o.auth.installations(), allowed, desired, pushedAt, &stats)...)

	idx := listIndexed(s.o)
	present := map[string]bool{}
	for _, x := range idx {
		present[x.Repo] = true
	}

	// Skip repositories that already have a job in flight so batched syncs make
	// forward progress instead of re-enqueuing the same repos each cycle. Enqueue
	// at most reconcileBatch new repos per cycle (0 = unlimited) so large orgs
	// sync gradually across cycles rather than blasting thousands of clones at
	// once and tripping rate limits.
	inflight := s.q.pendingRepos()
	queued, deferred := selectReposToEnqueue(desired, present, inflight, s.o.reconcileBatch)
	for _, repo := range queued {
		s.q.enqueue("watch", "POST", desired[repo])
	}

	// Freshness safety net: webhooks are the primary freshness signal, but
	// GitHub does not retry failed deliveries, so a missed push would leave a
	// repo stale forever. pushed_at comes free with discovery; re-index (PUT =
	// pull + reindex) every indexed repo pushed after its shard's indexedAt.
	// Refresh shares the cycle's batch budget, new repos first.
	remaining := 0
	if s.o.reconcileBatch > 0 {
		remaining = max(s.o.reconcileBatch-len(queued), 0)
	}
	stale, staleDeferred := selectStaleReposToRefresh(pushedAt, idx, inflight, remaining)
	for _, repo := range stale {
		s.q.enqueue("refresh", http.MethodPut, desired[repo])
	}

	pruned, pruneErrs := pruneUndesired(s.o, idx, desired, len(errorsOut) == 0)
	errorsOut = append(errorsOut, pruneErrs...)

	// One structured summary per cycle (never per repo). Errors are logged
	// separately at error level so they surface even in production.
	s.log().Info("reconcile",
		zap.Int("discovered", stats.discovered),
		zap.Int("archived", stats.archived),
		zap.Int("disabled", stats.disabled),
		zap.Int("otherOrg", stats.otherOrg),
		zap.Int("filtered", stats.filtered),
		zap.Int("desired", len(desired)),
		zap.Int("present", len(present)),
		zap.Int("pruned", len(pruned)),
		zap.Int("queued", len(queued)),
		zap.Int("deferred", deferred),
		zap.Int("refreshed", len(stale)),
		zap.Int("refreshDeferred", staleDeferred),
		zap.Int("errors", len(errorsOut)),
	)
	for _, detail := range errorsOut {
		s.log().Error("reconcile source failed", zap.String("detail", detail))
	}
	return map[string]any{
		"discovered":      stats.discovered,
		"archived":        stats.archived,
		"disabled":        stats.disabled,
		"otherOrg":        stats.otherOrg,
		"filtered":        stats.filtered,
		"desired":         len(desired),
		"present":         len(present),
		"pruned":          pruned,
		"queued":          queued,
		"deferred":        deferred,
		"refreshed":       len(stale),
		"refreshDeferred": staleDeferred,
		"batch":           s.o.reconcileBatch,
		"errors":          errorsOut,
		"rebalanced":      rebalance(s.o, false),
	}
}

// handleReconcile runs one reconcile cycle on demand.
func (s *routerState) handleReconcile(c fiber.Ctx) error {
	return writeJSON(c, fiber.StatusOK, s.reconcile())
}

// handleNotFound is the terminal handler for unmatched routes.
func (s *routerState) handleNotFound(c fiber.Ctx) error {
	return errorJSON(c, fiber.StatusNotFound, "not found")
}

// Run is the router entry point: it serves the control plane (reconcile,
// webhooks, CLI auth/search fan-out, and shard management) until the process
// is signaled. args excludes argv[0].
func Run(args []string) error {
	options, err := parseArgs(args)
	if err != nil || options == nil {
		return err
	}
	logger := newLogger()
	defer func() { _ = logger.Sync() }()
	state := &routerState{o: *options, logger: logger}
	state.o.logger = logger
	state.q = newQueue(*options)
	// api.Rule definitions are the router's own state; persist them (mount a volume in
	// production) and recover from the shards if the file was lost on restart.
	state.rules = newRuleRegistry(environment("GREPPLE_RULES_FILE", "rules.json"), *options, logger)
	state.rules.recoverFromBackends()
	go state.rules.distribute()
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", options.host, options.port))
	if err != nil {
		return err
	}
	logger.Info("router listening",
		zap.String("addr", fmt.Sprintf("%s:%d", options.host, listener.Addr().(*net.TCPAddr).Port)),
		zap.Int("backends", len(options.backends)),
	)
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go state.reconcileLoop(shutdown)
	return state.handler().Listener(listener, fiber.ListenConfig{
		DisableStartupMessage: true,
		GracefulContext:       shutdown,
		ShutdownTimeout:       10 * time.Second,
	})
}

// --- HTTP handlers (CLI-facing, under /public) ---

func (s *routerState) handleRuleCreate(c fiber.Ctx) error {
	if s.rules == nil {
		return errorJSON(c, fiber.StatusServiceUnavailable, "rules not initialized")
	}
	var rule api.Rule
	if err := json.Unmarshal(c.Body(), &rule); err != nil {
		return errorJSON(c, fiber.StatusBadRequest, "invalid JSON body")
	}
	stored, err := s.rules.upsert(rule)
	if err != nil {
		return errorJSON(c, fiber.StatusBadRequest, err)
	}
	// Push the new set to the shards and kick off their backfill before replying,
	// so the rule is being materialized by the time the caller polls results.
	go s.rules.distribute()
	return writeJSON(c, fiber.StatusCreated, stored)
}

func (s *routerState) handleRuleList(c fiber.Ctx) error {
	if s.rules == nil {
		return writeJSON(c, fiber.StatusOK, api.RuleSet{})
	}
	return writeJSON(c, fiber.StatusOK, s.rules.ruleSet())
}

func (s *routerState) handleRuleGet(c fiber.Ctx) error {
	if s.rules == nil {
		return errorJSON(c, fiber.StatusNotFound, "no such rule")
	}
	rule, ok := s.rules.get(c.Params("id"))
	if !ok {
		return errorJSON(c, fiber.StatusNotFound, "no such rule: "+c.Params("id"))
	}
	return writeJSON(c, fiber.StatusOK, rule)
}

func (s *routerState) handleRuleDelete(c fiber.Ctx) error {
	if s.rules == nil {
		return errorJSON(c, fiber.StatusNotFound, "no such rule")
	}
	if !s.rules.remove(c.Params("id")) {
		return errorJSON(c, fiber.StatusNotFound, "no such rule: "+c.Params("id"))
	}
	go s.rules.distribute()
	return writeJSON(c, fiber.StatusOK, fiber.Map{"id": c.Params("id"), "removed": true})
}

func (s *routerState) handleRuleResults(c fiber.Ctx) error {
	if s.rules == nil {
		return errorJSON(c, fiber.StatusNotFound, "no such rule")
	}
	results, err := s.rules.fetchResults(c.Params("id"))
	if err != nil {
		return errorJSON(c, fiber.StatusNotFound, err)
	}
	return writeJSON(c, fiber.StatusOK, results)
}

// reconcileCycle runs a single reconcile (when repos/orgs are watched) or a
// standalone rebalance pass otherwise.
func (s *routerState) reconcileCycle() {
	if len(s.o.watchRepos) > 0 || len(s.o.watchOrgs) > 0 || s.o.auth.hasInstallations() {
		s.reconcile()
	} else {
		_ = rebalance(s.o, false)
	}
	// Re-push the ruleset so a shard that restarted (or was unreachable) catches
	// up. Shards apply idempotently by generation, so this is a cheap no-op when
	// nothing changed.
	if s.rules != nil {
		s.rules.distribute()
	}
}

// reconcileLoop performs the startup reconcile immediately and then repeats it
// on reconcileInterval until the context is cancelled. Periodic reconciliation
// lets shards that were unreachable during startup recover: their missing repos
// are re-enqueued once the shard is reachable again, preventing permanently
// unbalanced shards. An interval of zero disables periodic reconciliation.
func (s *routerState) reconcileLoop(ctx context.Context) {
	s.reconcileCycle()
	if s.o.reconcileInterval <= 0 {
		return
	}
	ticker := time.NewTicker(s.o.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileCycle()
		}
	}
}

// handleRepos lists every repository indexed across the shards so the CLI
// (`grepple repos`) can discover exact OWNER/REPO names without a throwaway
// --count probe. It mirrors the repo set surfaced on the root endpoint but is
// served under /public so it shares the org-auth guard with get/tree.
func (s *routerState) handleRepos(c fiber.Ctx) error {
	indexed := listIndexed(s.o)
	repos := make([]fiber.Map, 0, len(indexed))
	for _, info := range indexed {
		repos = append(repos, fiber.Map{
			"repo":      info.Repo,
			"shard":     info.Shard,
			"ref":       info.Ref,
			"head":      info.Head,
			"indexedAt": info.IndexedAt,
		})
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i]["repo"].(string) < repos[j]["repo"].(string)
	})
	return writeJSON(c, fiber.StatusOK, fiber.Map{"ok": true, "count": len(repos), "repos": repos})
}
