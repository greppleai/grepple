package router

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"go.uber.org/zap"
)

type routerOptions struct {
	host                  string
	port                  int
	backends              []string
	timeout               time.Duration
	webhookSecret         string
	concurrency           int
	githubToken           string
	auth                  *githubAuth
	watchRepos, watchOrgs []string
	reconcileInterval     time.Duration
	reconcileBatch        int
	logger                *zap.Logger
	githubClientID        string
	githubClientSecret    string
	githubWebBase         string
	githubScopes          string
	githubAPIBase         string
	authRequired          bool
	authOrgs              []string
	ring                  *hashRing
}

type routerArgs struct {
	Host               string   `arg:"--host" placeholder:"HOST" help:"bind host"`
	Port               int      `arg:"--port" placeholder:"PORT" help:"bind port"`
	Backends           []string `arg:"--backend,separate" placeholder:"URL" help:"shard backend; repeatable"`
	Timeout            int      `arg:"--timeout" placeholder:"MILLISECONDS" help:"backend timeout"`
	WebhookSecret      string   `arg:"--webhook-secret" placeholder:"SECRET" help:"GitHub webhook secret"`
	WebhookConcurrency int      `arg:"--webhook-concurrency" placeholder:"N" help:"concurrent index jobs"`
	GitHubToken        string   `arg:"--github-token" placeholder:"TOKEN" help:"GitHub API and clone token (personal access token; handy for local testing)"`
	GitHubAppConfig    string   `arg:"--github-app-config" placeholder:"FILE" help:"path to GitHub App JSON config (app id + private key + installations)"`
	WatchRepos         []string `arg:"--watch,separate" placeholder:"OWNER/REPOSITORY" help:"repository to reconcile; repeatable"`
	WatchOrgs          []string `arg:"--watch-org,separate" placeholder:"ORGANIZATION" help:"organization to reconcile; repeatable"`
	ReconcileInterval  int      `arg:"--reconcile-interval" placeholder:"MILLISECONDS" help:"periodic reconcile interval; 0 disables"`
	ReconcileBatch     int      `arg:"--reconcile-batch" placeholder:"N" help:"max new repos enqueued per reconcile cycle; 0 = unlimited"`
	GitHubClientID     string   `arg:"--github-client-id" placeholder:"ID" help:"GitHub OAuth/App client ID advertised to CLIs via /auth/config for device-flow login"`
	GitHubClientScopes string   `arg:"--github-client-scopes" placeholder:"SCOPES" help:"OAuth scopes advertised for login (default read:user)"`
}

func (routerArgs) Description() string {
	return "Run the distributed search router and control plane."
}

func normalizeBackend(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}

func parseArgs(args []string) (*routerOptions, error) {
	values := routerDefaults()
	parser, err := arg.NewParser(arg.Config{Program: "router"}, &values)
	if err != nil {
		return nil, err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return nil, nil
		}
		return nil, err
	}
	if err := validateRouterArgs(&values); err != nil {
		return nil, err
	}
	backends, err := normalizeBackends(values.Backends)
	if err != nil {
		return nil, err
	}
	apps, err := loadAppConfigs(values.GitHubAppConfig)
	if err != nil {
		return nil, err
	}
	apiBase := environment("GITHUB_API_URL", "https://api.github.com")
	auth, err := buildGithubAuth(values.GitHubToken, apps, apiBase)
	if err != nil {
		return nil, err
	}
	requireAuth := boolEnv("GREPPLE_REQUIRE_AUTH", false)
	return &routerOptions{
		host:               values.Host,
		port:               values.Port,
		backends:           backends,
		timeout:            time.Duration(values.Timeout) * time.Millisecond,
		webhookSecret:      values.WebhookSecret,
		concurrency:        values.WebhookConcurrency,
		githubToken:        values.GitHubToken,
		auth:               auth,
		watchRepos:         values.WatchRepos,
		watchOrgs:          values.WatchOrgs,
		reconcileInterval:  time.Duration(values.ReconcileInterval) * time.Millisecond,
		reconcileBatch:     values.ReconcileBatch,
		githubClientID:     values.GitHubClientID,
		githubClientSecret: githubClientSecret(),
		githubWebBase:      environment("GREPPLE_GITHUB_HOST", "https://github.com"),
		githubScopes:       values.GitHubClientScopes,
		githubAPIBase:      apiBase,
		authRequired:       requireAuth,
		authOrgs:           parseCSV(os.Getenv("GREPPLE_AUTH_ORGS")),
		ring:               newRing(backends),
	}, nil
}

// routerDefaults builds the flag values from the environment before CLI flags
// override them.
func routerDefaults() routerArgs {
	port, _ := strconv.Atoi(envValue("GREPPLE_PORT", "8080"))
	timeout, _ := strconv.Atoi(envValue("GREPPLE_TIMEOUT", "10000"))
	concurrency, _ := strconv.Atoi(envValue("GREPPLE_WEBHOOK_CONCURRENCY", "4"))
	reconcileInterval, _ := strconv.Atoi(envValue("GREPPLE_RECONCILE_INTERVAL", "300000"))
	reconcileBatch, _ := strconv.Atoi(envValue("GREPPLE_RECONCILE_BATCH", "50"))
	values := routerArgs{
		Host:               envValue("GREPPLE_HOST", "0.0.0.0"),
		Port:               port,
		Timeout:            timeout,
		WebhookSecret:      envValue("GREPPLE_WEBHOOK_SECRET", ""),
		WebhookConcurrency: concurrency,
		GitHubToken:        os.Getenv("GITHUB_TOKEN"),
		WatchRepos:         parseCSV(os.Getenv("WATCH_REPOS")),
		WatchOrgs:          parseCSV(os.Getenv("WATCH_ORGS")),
		ReconcileInterval:  reconcileInterval,
		ReconcileBatch:     reconcileBatch,
		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientScopes: envValue("GITHUB_CLIENT_SCOPES", "read:org"),
	}
	values.Backends = append(values.Backends, parseCSV(os.Getenv("SHARD_HOSTS"))...)
	values.Backends = append(values.Backends, parseCSV(envValue("GREPPLE_BACKENDS", ""))...)
	return values
}

// validateRouterArgs rejects out-of-range numeric flags.
func validateRouterArgs(values *routerArgs) error {
	if values.Port < 0 || values.Port > 65535 {
		return fmt.Errorf("--port must be a valid port number")
	}
	if values.Timeout <= 0 {
		return fmt.Errorf("--timeout must be a positive number")
	}
	if values.WebhookConcurrency < 1 {
		return fmt.Errorf("--webhook-concurrency must be a positive number")
	}
	if values.ReconcileInterval < 0 {
		return fmt.Errorf("--reconcile-interval must not be negative")
	}
	if values.ReconcileBatch < 0 {
		return fmt.Errorf("--reconcile-batch must not be negative")
	}
	return nil
}

// normalizeBackends normalizes, dedupes, and validates the backend URLs; the
// router cannot run with zero shards.
func normalizeBackends(values []string) ([]string, error) {
	seen := map[string]bool{}
	var backends []string
	for _, value := range values {
		backend := normalizeBackend(value)
		if backend == "" || seen[backend] {
			continue
		}
		parsed, err := url.Parse(backend)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("invalid backend URL %q", value)
		}
		seen[backend] = true
		backends = append(backends, backend)
	}
	if len(backends) == 0 {
		return nil, fmt.Errorf("router requires at least one shard (SHARD_HOSTS, GREPPLE_BACKENDS, or --backend)")
	}
	return backends, nil
}

// githubClientSecret reads the GitHub App client secret from an env var or a
// mounted file. It is needed only to exchange refresh tokens for new user
// access tokens (/auth/refresh) and never leaves the router.
func githubClientSecret() string {
	if secret := environment("GITHUB_CLIENT_SECRET", ""); secret != "" {
		return secret
	}
	if file := os.Getenv("GITHUB_CLIENT_SECRET_FILE"); file != "" {
		if b, err := os.ReadFile(file); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

func envValue(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func environment(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// boolEnv reads a boolean environment variable (1/true/yes/on), returning
// fallback when unset or unparseable.
func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// allowedAuthOrgs returns the lowercased set of organizations whose members may
// call the authenticated CLI endpoints. It uses GREPPLE_AUTH_ORGS when set,
// otherwise derives the set from configured discovery (App installations,
// watched orgs, and the owners of watched repositories).
func (o routerOptions) allowedAuthOrgs() map[string]bool {
	orgs := map[string]bool{}
	add := func(org string) {
		if org = strings.TrimSpace(strings.ToLower(org)); org != "" {
			orgs[org] = true
		}
	}
	if len(o.authOrgs) > 0 {
		for _, org := range o.authOrgs {
			add(org)
		}
		return orgs
	}
	for _, inst := range o.auth.installations() {
		add(inst.org)
	}
	for _, org := range o.watchOrgs {
		add(org)
	}
	for _, repo := range o.watchRepos {
		add(orgOf(repo))
	}
	return orgs
}
