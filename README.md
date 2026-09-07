# grepple

Structure-aware grep for agents, implemented in Go. Search works across text files, while supported programming-language files receive compact tree-sitter structural context. Results are returned in deterministic path order (there is no relevance ranking) so you refine by narrowing with filters rather than trusting a score.

## Project layout

```text
.grepple/       Canonical machine-generated architecture artifacts
  *.package/     Package manifests, overviews, and exhaustive structures
  *.workspace/   Project-level manifests and overviews
  *.flow.mmd      Generated call-flow schemas
cmd/
  grepple/     Search/client CLI
  shard/       Repository search shard
  router/      Distributed router and control plane
docs/
  file-type-support.md  File classification and language-extension guide
hooks/         Standalone Go module for replaceable project-local pi hooks
  cmd/pi-hook/      Hook protocol entry point
  cmd/mermaid-code/ Mermaid code-schema checker and generator
  internal/pihooks/
  internal/mermaidcode/ Tree-sitter Go and TypeScript diagram analysis
  guides/           Agent remediation guides
internal/
  api/         Dependency-free HTTP request and response contracts
  command/     Shared process/error handling
  grepplecli/  Grepple-only CLI arguments and local/remote workflows
  parser/      Language detection, tree-sitter parsing, segments, and outlines
  search/      File discovery, matching, filtering, paging, and result construction
  shard/       Fiber v3 shard API, CLI arguments, and Zoekt integration
  router/      Fiber v3 router API, jobs, webhooks, and reconciliation
  repository/  Git checkout registry and safe repository filesystem access
```

All Go source is formatted with `gofmt`. The project requires Go 1.25 for Fiber v3. Tree-sitter uses CGO, so builds also require a C compiler.

## Build and install

Prebuilt static `grepple` CLI binaries for Linux (amd64 and arm64, with sha256 checksums) are attached to every GitHub release. Releases and the changelog are automated with [release-please](https://github.com/googleapis/release-please); use [conventional commits](https://www.conventionalcommits.org/) (e.g. `feat:`, `fix:`) so changes are picked up for the next release.

To build from source:

```bash
go test ./...
make build
make install
```

Linting uses [revive](https://github.com/mgechev/revive) with the pinned rule set in `revive.toml` (the documented default rules, made explicit, plus `cognitive-complexity` capped at 15):
`make lint` runs revive plus the standalone pi hook module's lint and tests. Pi executes a cached `hooks/bin/pi-hook` binary; the lightweight Make targets in `.pi/settings.json` rebuild it only when its Go sources or module files change. The Stop hook runs
on every agent Stop: it auto-fixes formatting with `gofmt -w`, then feeds one revive rule-group (worst file first, one
rule at a time) back to the agent with a remediation guide from `hooks/guides/<rule>.md` until the tree is clean.
Project analyzers in `hooks/internal/pihooks/` add repo-specific checks that revive cannot express. The current
`same-file-struct-methods` analyzer uses tree-sitter Go syntax trees and requires every method of a struct to live in the file that declares the type.
The PreToolUse grep guard is also implemented in the standalone module and uses the tree-sitter Bash grammar to inspect actual command invocations.

The hooks module validates and generates Mermaid class/structure and call-flow schemas from Go and TypeScript/TSX. This repository exports every generated architecture artifact to the root [`.grepple/`](.grepple/) directory; `make schema-generate` rewrites them there and `make schema-check` reads and verifies them there. Go package generation supports both the legacy single diagram and canonical three-file bundles: `mermaid-code generate package <source-directory> --format bundle --output <bundle-directory>` and `mermaid-code check package <bundle-directory> [source-directory]`. It also generates canonical two-file project inventories with `mermaid-code generate workspace <root> --output <bundle-directory>` and validates them with `mermaid-code check workspace <bundle-directory> [root]`. Bundle manifests carry normalized project-relative source metadata, so checks can self-locate their source. The Stop hook automatically validates legacy `*.class.mmd`, `*.structure.mmd`, and `*.flow.mmd` schemas plus canonical `*.package/manifest.json` and `*.workspace/manifest.json` bundles. Generated output is excluded from source analysis, while Stop discovery still enters `.grepple/` to validate it. Generation and checking use `hooks/bin/mermaid-code`. See [`hooks/README.md`](hooks/README.md) for commands, bundle artifacts, metadata, and discovery rules.

The three application binaries are built into `bin/`:

- `grepple`
- `shard`
- `router`

Shard builds also install the required `zoekt-git-index` and `zoekt-webserver` helper executables into `bin/`.

The default Linux build links the CGO tree-sitter runtime and all grammars into self-contained static binaries. Command-line parsing uses [`go-arg`](https://github.com/alexflint/go-arg).

## Search

```bash
grepple "console\\.log" "src/*.ts"
grepple -F "console.log" src
grepple --files "config/*.yaml" "config/*.yml"
grepple --count "httpRoute"
```

Supported search options:

- `--regex` (default), `-F`/`--fixed-strings`, `-i`/`--ignore-case`
- `-n`/`--line-number`, `--line-only`, `-C`/`--context N`
- `-l`/`--files` (list files whose **path** matches the glob), `--files-with-matches` (list paths of files whose **contents** match, like `grep -l`), `-c`/`--count`
- `--json`, `--json-matches`
- `--repo PATTERN` (repeatable)
- `--max-files N`, `--max-segments N`
- `--skip N`, `--limit N` (page through results in deterministic path order: skip the first `N` files / return at most `N`; **`--limit` defaults to `20`**, use `--limit 0` for all). A server never returns more than **100 files per page** — `--limit 0` or a larger value gets the maximum page, and you page further with `--skip N`; local-only searches stay uncapped

File patterns use Go's `filepath.Glob` syntax, extended with `**` to match across directory boundaries (for example `**/*.yaml` or `charts/**/values.yaml`). Omit globs to search recursively from the working directory; a matched directory is also searched recursively. `.git` directories and repository `.gitignore` entries are excluded. Shard searches are confined to the served repository root, so client-supplied globs and paths cannot escape it. Globs compose with every output mode, including `-c`/`--count`. When a Zoekt index is available the globs are translated into a `file:` atom and pushed down to the index (a deliberate superset — the shard still applies the exact glob matcher to what the index returns), so the index pre-filters by path instead of shipping every content match for the shard to discard.

**Pipes work like `grep`/`rg`:** when standard input is piped (or redirected) and no path/glob argument is given, grepple searches the stream instead of the filesystem — `cat build.log | grepple "ERROR"` or `go test ./... | grepple -F "FAIL"`. The stream is reported under the virtual path `<stdin>` in every output mode (`--json`, `-c`, `--files-with-matches`, `--line-only`, default segments). It uses the plain-text fallback (no filename, so no tree-sitter structure), NUL-containing input is treated as binary and skipped, and no match exits 1 as usual. `--files` and `--outline` always operate on the filesystem, and passing any path/glob selects the filesystem over stdin.

## Outline

`--outline` (`-O`) prints a file's structural map instead of searching its contents:
for code its classes, interfaces, functions, methods, structs/types and fields; for
**JSON/YAML** the key/shape skeleton with values omitted — each with the line ranges
they span. It is a generated-header / `ctags`-style view for quickly orienting in a
file, and is especially useful on **large** config/data files where you want the
shape, not the payload. Code reuses the same tree-sitter parsing as search; JSON/YAML
are parsed with yaml.v3.

```bash
grepple --outline internal/search/rules.go     # one local file
grepple --outline "internal/**/*.go"           # every file matching the glob
grepple --outline --json src/app.ts            # machine-readable {"files":[...]}
grepple --outline values.yaml                  # JSON/YAML: key tree, values omitted
grepple --outline --depth 2 large-config.json  # cap nesting (JSON/YAML only)
grepple get OWNER/REPO path/to/File.java -O    # a remote indexed file
```

Output is `START-END<TAB>KIND<TAB>NAME`, with members indented under their
container:

```
internal/router/rules.go	go
20-28	struct	ruleRegistry
112-130	method	(*ruleRegistry).upsert
216-262	method	(*ruleRegistry).fetchResults
```

The tree-sitter languages (Go, TypeScript/TSX, JavaScript/JSX, Java, Kotlin) get
full symbol outlines; Markdown gets a heading outline (`h1`…`h6`, code-fence aware);
**JSON/YAML** get a key/type tree (`object`/`array`/`string`/`number`/`bool`/`null`)
with values omitted — arrays show a `[N]` length and recurse only into object/array
elements (scalar lists collapse), scalars are typed via the resolved YAML tag, and
**multi-document YAML** (`---`) is supported as `document [i]` nodes. Other file types
produce an empty outline. Go methods are shown at top level as `(*Type).Method`.
`--outline` composes with `--repo`/`--limit`/`--skip` (and `--depth N` for JSON/YAML,
0 = unlimited) and cannot be combined with `--count` or `--files-with-matches`.
## Predefined searches (rules)

A **rule** is a saved search whose result is materialized per repository and
refreshed incrementally whenever a repository is reindexed — conceptually a
Prometheus recording rule, but over source code. It makes recurring org-wide
questions ("which repos use a given GitHub Action / base image / dependency?")
cheap to answer: fetching a rule's results reads materialized state instead of
re-scanning the corpus.

```bash
# Create a rule (add takes the same options as search; mode is count or files).
grepple rules add --name "Uses Checkout" "uses: actions/checkout" ".github/workflows/*.yml"
grepple rules list
grepple rules results uses-checkout          # per-repo tally (--json for machine output)
grepple rules rm uses-checkout
```

How it works:

- The **router** is the source of truth for definitions (persisted to `rules.json`,
  path overridable via `GREPPLE_RULES_FILE`; mount a volume in production). It pushes
  a generation-stamped ruleset to every shard and, if its file is lost, recovers the
  set from the shards.
- Each **shard** evaluates rules for the repos it owns and stores the results
  (`rules.json` on the shard). On a repo reindex it re-evaluates that one repo; on a
  rule change it backfills across all its repos.
- `GET /public/rules/:id/results` aggregates the per-repo rows across shards.

Results are eventually consistent: each repo's row is stamped with the commit it
reflects (`head`) and when it was computed (`evaluatedAt`). Modes: `count` (per-repo
`{files, matches}`) and `files` (matching paths, capped per repo). The `POST
/public/rules` body embeds a full search request, so any option the CLI/search API
supports is available to a rule.

REST API (all under the org-authed `/public` gate):

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/public/rules` | create/replace a rule (body: `{name, mode, request, id?}`) |
| GET | `/public/rules` | list definitions (+ generation) |
| GET | `/public/rules/:id` | one definition |
| DELETE | `/public/rules/:id` | delete a rule |
| GET | `/public/rules/:id/results` | aggregated per-repo results |

## File type support

Matching is line-based for every text file. File extensions determine whether tree-sitter can additionally provide structural result segments (this shapes output, not ordering — there is no relevance ranking).

| Language | Extensions | Structural behavior |
| --- | --- | --- |
| TypeScript | `.ts` | Functions, classes, interfaces, types, methods, variables, imports, and exports |
| TSX | `.tsx` | TypeScript structure plus focused JSX element context |
| JavaScript | `.js`, `.jsx` | Functions, classes, methods, variables, imports, exports, and JSX |
| Go | `.go` | Functions, methods, types, variables, constants, and imports |
| Java | `.java` | Classes, interfaces, enums, records, constructors, methods, fields, and imports |
| Kotlin | `.kt`, `.kts` | Classes, objects, functions, properties, and imports |
| Markdown | `.md`, `.markdown`, `.mdown`, `.mkd` | Heading sections — a match is shown under its enclosing heading chain (`#`…`######` breadcrumb); also powers `--outline` |
| JSON | `.json` | `--outline` only: key/shape skeleton with values omitted (content search uses the plain-text fallback) |
| YAML | `.yaml`, `.yml` | `--outline` only: key/shape skeleton, typed scalars, multi-document (`---`) aware (content search uses the plain-text fallback) |
| Other text | Any other extension or no extension | Matching lines without AST-derived structure |

Markdown uses a lightweight heading scanner (not a tree-sitter grammar) to give matches their enclosing section context and to build outlines. JSON and YAML are parsed for `--outline` via `go.yaml.in/yaml/v3` (JSON is valid YAML), which preserves line numbers and supports multi-document YAML streams; their *content* search still uses the plain-text fallback. Shell scripts, configuration files, and other textual formats are searchable through the plain-text fallback. During content searches, files containing a NUL byte are treated as binary and skipped, and invalid UTF-8 bytes are replaced during matching. `--files` reports discovered paths without opening them, so it can include binary or unreadable files. Extensions such as `.mjs`, `.cjs`, `.mts`, and `.cts` currently use the plain-text fallback.

See [`docs/file-type-support.md`](docs/file-type-support.md) for implementation details and instructions for adding another tree-sitter-backed file type.

## Distributed services

Start the complete local two-shard cluster:

```bash
cp .env.example .env
# Edit WATCH_REPOS, WATCH_ORGS, and GITHUB_TOKEN as needed.
./start-local.sh
```

This starts a loopback-only router on `localhost:8080`, shard debug endpoints on `127.0.0.1:8787` and `:8788`, and one persistent repository volume per shard. Ports can be changed with `GREPPLE_ROUTER_PORT`, `GREPPLE_SHARD_A_PORT`, and `GREPPLE_SHARD_B_PORT`. Startup waits until the router and both shards are healthy. Use `./start-local.sh logs -f`, `./start-local.sh down`, or `./start-local.sh down -v`.

Run the dedicated server binaries:

```bash
shard --root /srv/repos --port 8787
router --port 8080 \
  --backend http://shard-a:8787 \
  --backend http://shard-b:8787
```

Search is **local-first**: it only queries a remote server when you opt in with
`--remote` (`-R`) or by passing `--server`. A configured `GREPPLE_SERVER`/config
`server` just supplies the URL to use when you opt in.

```bash
grepple "useEffect" "src/*.tsx"                    # local only (default)
grepple --remote "useEffect" "src/*.tsx"          # local + configured remote
grepple --server http://localhost:8080 "useEffect"  # explicit server (implies --remote)
grepple repos                                     # list indexed OWNER/REPO names (remote)
grepple get owner/repo path/to/file --lines 20:50
grepple tree owner/repo src --depth 3
```

The three commands are intentionally independent: `grepple` does not parse shard/router options or import their implementations. Zoekt is shard-only.

Shard endpoints: `/health`, `/search`, `/index`, `/raw`, and `/tree`. The router's CLI-facing endpoints live under the `/public` prefix (`/public/search`, `/public/raw`, `/public/tree`, `/public/repos`) and are guarded by an authentication middleware; control-plane endpoints (`/health`, `/livez`, `/`, `/jobs`, `/backends`, `/index`, `/reconcile`, `/rebalance`, `/github/webhook`) and `/auth/config` are unauthenticated. The gateway (HTTPRoute) exposes only the externally-facing surface — `/public/*`, `/github/webhook`, and `/auth/config` — so the control-plane endpoints are reachable only in-cluster. The GitHub webhook is publicly routed but never requires a bearer token: it is authenticated by its HMAC signature, so it sits outside the `/public` auth group. Both HTTP services use Fiber v3 with panic recovery, a 32 MiB request-body limit, 64 KiB request buffers, bounded read/idle timeouts, and ten-second graceful shutdown. The router uses Fiber's proxy middleware for `/public/raw` and `/public/tree` (rewriting the outbound path to the shard's `/raw`/`/tree`), preserves configured backend path prefixes, and limits buffered backend responses to 64 MiB.

Shard startup is asynchronous so a shard becomes ready in seconds regardless of how many repositories it holds. Each shard persists `directory.json` (a list of `repo` + last-indexed `head` sha) alongside its checkouts. On boot the shard trusts the existing Zoekt shards on disk, marks their coverage as known so searches use them immediately, starts serving, and then reconciles the index in the background: it re-indexes only repositories whose head changed (or whose shard is missing) and computes accurate scanner-fallback coverage. The index is therefore eventually consistent — a just-started shard may briefly omit repositories that changed while it was down until the background pass reaches them. `directory.json` is updated whenever a repository is (re)indexed or removed.
Router endpoints additionally include `/`, `/livez`, `/backends`, `/jobs`, `/github/webhook`, `/reconcile`, and `/rebalance`. The `/github/webhook` endpoint (HMAC-verified with `GREPPLE_WEBHOOK_SECRET` when set) reacts to `push` (re-index the pushed branch), and to `repository` events `created` (clone/index), `deleted` and `archived` (remove from the index), and `unarchived` (re-clone) — archiving a repo therefore drops it promptly rather than waiting for the next reconcile. Both probe endpoints report only the router's own state and never probe the shards, so scaling shards or a single starting/down shard never makes the router itself un-ready: `/livez` (liveness) is dependency-free, and `/health` (readiness) always returns `200` while the router is running. Shard health is available separately on `/backends` (informational; returns `503` when a shard is unreachable so dashboards can alert), and job-queue stats only on `/jobs` (no longer duplicated into `/` or `/health`).

The router reconciles once at startup and then periodically. The interval is set with `--reconcile-interval` or `GREPPLE_RECONCILE_INTERVAL` (milliseconds, default `300000`; `0` disables periodic reconciliation). Periodic reconciliation is what lets a shard that was unreachable during startup recover on its own: its repositories are re-enqueued to the correct shard once the shard is reachable again, so a transient startup failure no longer leaves the shards permanently unbalanced.

Each reconcile cycle enqueues at most `--reconcile-batch` / `GREPPLE_RECONCILE_BATCH` new repositories (default `50`; `0` = unlimited), and repositories that already have a job queued or running are skipped. This paces the initial sync of a large organization (thousands of repositories) across successive cycles instead of dispatching every clone at once and tripping GitHub rate/abuse limits — a full sync completes over roughly `ceil(repos / batch)` cycles. Reduce the batch (or lengthen the interval) to sync more gently; set the batch to `0` to restore the eager behavior.

Reconcile is also the **freshness safety net**: content updates are normally driven by push webhooks, but GitHub does not retry failed webhook deliveries, so a missed push would leave a repo stale until its next push. Discovery payloads carry GitHub's `pushed_at` for free, so each cycle re-indexes (`PUT` = pull + reindex) every indexed repository whose `pushed_at` is newer than its shard's `indexedAt` — staleness self-heals within one interval, no extra API calls. Refreshes share the cycle's batch budget (new repos first). Repositories reachable only via the `WATCH_REPOS` allowlist carry no `pushed_at` and rely on webhooks alone.

Reconcile also **prunes**: any indexed repository that is no longer in the desired set — archived, disabled, deleted, transferred, or made inaccessible to the installation — is removed from the shard that holds it. This is the backstop for missed webhook deliveries and keeps the index from growing unbounded. Pruning is gated for safety: it is skipped when any discovery source errored or when the desired set is empty, so a transient GitHub API failure can never wipe the index. The `/reconcile` response and job stats report `queued`, `deferred` (repositories held back for later cycles), `refreshed` (stale repositories re-indexed this cycle), `refreshDeferred`, `pruned` (repositories removed this cycle), and `batch`, plus the discovery counters `discovered`, `archived`, `disabled`, and `otherOrg` (why discovered repos were excluded from the desired set).

### GitHub authentication and discovery

The router discovers repositories from `WATCH_REPOS` (explicit `owner/name` list) and `WATCH_ORGS` (whole organizations), and clones them over HTTPS. When `WATCH_REPOS` is **non-empty it acts as a hard allowlist**: only those repositories are indexed, and organization/GitHub-App discovery is narrowed to that set — identically for PAT and App auth (discovery is still used to resolve each allowed repo's clone URL and default branch, and an allowlisted repo that discovery does not return is still cloned via its default GitHub URL). When `WATCH_REPOS` is empty, all discovered repositories are indexed. Repositories excluded by the allowlist are reported as `filtered` in the `/reconcile` response and the reconcile log summary. Authentication resolves per repository owner:

- **GitHub App installations (recommended for production).** Provide an app id, its RSA private key, and one or more installations (each an `org` + `installationId`). The router signs a short-lived RS256 JWT as the app, exchanges it for an installation access token per installation (cached and refreshed shortly before its ~1h expiry), discovers that installation's repositories via `GET /installation/repositories` (scoped to the installation's `org`), and uses the installation token for both discovery and cloning. You can configure **multiple app + org pairs** and/or a **single app with multiple installation ids** (for an enterprise with one installation per org).
- **Personal access token (handy for local testing).** `GITHUB_TOKEN` is used for `WATCH_ORGS` discovery and as the fallback clone token for any repository whose owner is not covered by a configured App installation. It remains fully supported alongside App auth.

For each repository the router picks the App installation token whose `org` matches the repository owner, falling back to `GITHUB_TOKEN` otherwise.

Configure GitHub Apps with either a JSON document or the flat single-app environment variables:

```jsonc
// GITHUB_APP_CONFIG (inline) or the file at GITHUB_APP_CONFIG_FILE / --github-app-config.
// A bare JSON array of the app objects is also accepted.
{
  "apps": [
    {
      "appId": 123456,
      "privateKeyFile": "/secrets/app-a.pem",   // or inline "privateKey": "-----BEGIN RSA PRIVATE KEY-----\n..."
      "installations": [
        { "org": "org-one", "installationId": 11111111 },
        { "org": "org-two", "installationId": 22222222 }
      ]
    },
    {
      "appId": 999999,
      "privateKey": "-----BEGIN RSA PRIVATE KEY-----\n...",
      "org": "org-three",           // single-installation shorthand
      "installationId": 33333333
    }
  ]
}
```

```bash
# Flat single-app form (merged with any JSON config above):
export GITHUB_APP_ID=123456
export GITHUB_APP_PRIVATE_KEY_FILE=/secrets/app-a.pem   # or GITHUB_APP_PRIVATE_KEY=<PEM contents>
export GITHUB_APP_INSTALLATIONS="org-one:11111111,org-two:22222222"
# ...or a single installation:
export GITHUB_APP_ORG=org-one
export GITHUB_APP_INSTALLATION_ID=11111111
```

Private keys may be PKCS#1 (`RSA PRIVATE KEY`) or PKCS#8 (`PRIVATE KEY`) PEM. `GITHUB_API_URL` overrides the REST base URL (for GitHub Enterprise Server). Router health reports `watch.githubAppInstallations` (the count of configured installations) alongside `watch.tokenConfigured`.

Search is **local-first**: a search runs only against the local working directory unless you opt into remote with `--remote` (`-R`) or by passing `--server` (which implies `--remote`). This keeps everyday use fast and offline and reaches the network only when asked. `--local` forces local-only (the default) and cannot be combined with `--remote`.

When a remote call is made, the server URL is resolved in this order:

1. `--server`
2. `GREPPLE_SERVER`
3. `./grepple.json`
4. `~/.grepple/config.json`
5. `http://127.0.0.1:8787` (final fallback)

When remote search runs and the working directory is inside a Git worktree, `grepple` walks upward to the nearest `.git`, derives `owner/name` from its `origin` (or first usable remote), and excludes that repository from remote search to avoid duplicate local/remote results. `get`, `tree`, and `rules` are inherently remote and always resolve a server (with the same `http://127.0.0.1:8787` final fallback).

### Login

`grepple login` authenticates with GitHub using the OAuth **device flow** and stores the resulting token (0600) in `~/.grepple/config.json`; `get`, `tree`, and `search` then send it as an `Authorization: Bearer` header. The client ID is **configured only on the server** (`GITHUB_CLIENT_ID`, a public identifier) and advertised at `GET /auth/config`; the CLI fetches it from the server, so users never configure a client ID locally:

```sh
grepple login --url https://grepple.example.com   # --url defaults to the configured server
grepple logout                                   # clears the stored token
```

`--scope` overrides the server's advertised scopes and `--no-browser` skips opening a browser. `GREPPLE_GITHUB_HOST`/`GREPPLE_GITHUB_API` point the device flow at GitHub Enterprise Server. Login fails loudly if the server advertises no client ID.

When the GitHub App is configured to expire user tokens (user access tokens last ~8h, with a ~6-month refresh token), the CLI **renews them silently** so you only log in about twice a year. `grepple login` stores the refresh token and expiry alongside the access token; before a request whose token is within five minutes of expiring, the CLI calls `POST /auth/refresh` on the server to exchange the refresh token for a fresh one and re-persists both. The exchange runs server-side because it needs the App **client secret** (`GITHUB_CLIENT_SECRET`, provided inline or via `GITHUB_CLIENT_SECRET_FILE`), which never leaves the router; `/auth/config` advertises whether refresh is available. If the secret is not configured the endpoint returns `501` and tokens simply expire as before. A refresh failure is non-fatal — the CLI falls back to the existing token and surfaces the normal re-login hint on a `401`. `GREPPLE_TOKEN` (an externally supplied token) is always used verbatim and never refreshed.

### Authorization

The router can require that callers of the CLI-facing `/public/*` endpoints authenticate. When `GREPPLE_REQUIRE_AUTH` is enabled, every `/public/search`, `/public/raw`, and `/public/tree` request must carry a `Bearer` token from `grepple login`; the router validates it against GitHub (`GET /user`) and, unless the caller is a member of an **authorized organization**, returns `403`. Authorized orgs come from `GREPPLE_AUTH_ORGS` (comma-separated) or, when unset, are derived from the router's own discovery configuration (GitHub App installations, `WATCH_ORGS`, and the owners of `WATCH_REPOS`). Membership is checked with the caller's own token via `GET /user/memberships/orgs/{org}`, so the advertised login scope defaults to `read:org`. Positively-validated tokens are cached for five minutes to keep the happy path off GitHub while ensuring revoked tokens and removed members lose access within the window. Auth is **off by default** (local development); enable it per environment. `/auth/config` and all control-plane endpoints remain unauthenticated.

File reading, matching, and tree-sitter result-segment construction run through a process-wide bounded worker pool. The default is `min(GOMAXPROCS, 8)`; set `GREPPLE_WORKERS` to a positive integer to override it. The shared limit prevents concurrent shard requests from multiplying parser concurrency.

grepple does not rank results — they are returned in deterministic path order (`owner/repo/dir/file`), stable across shards and reindexes, so precision comes from narrowing (with `--repo`, a tighter pattern, or `--count` to locate matches first), not from a score. Each returned file is tree-sitter-parsed at most once to build result segments; the window (`--skip`/`--limit`/`--max-files`) is applied first, so only the returned page is parsed and a small `--limit` bounds the work. Output modes that only need match lines — `-c`/`--count`, `-l`/`--files`, `--files-with-matches`, `--line-only`, `-C`/`--context N`, and `--json-matches` — skip the tree-sitter parse entirely (no structural segments are rendered); the CLI sends this hint to the shard so shards skip the work too. In distributed search each shard returns its first `skip+limit` files in path order and the router merges, de-duplicates, and pages the combined set (also in path order). When the underlying index hits its result cap it returns an incomplete but valid sample: the response is flagged `truncated` and the CLI prints a hint to narrow, rather than falling back to a slow full scan. Distributed search degrades gracefully: if some shards fail or time out (for example while a shard is busy indexing or restarting) the router still returns the results from the shards that responded and lists the failed shards in a `shardErrors` field (the CLI prints a `partial results` warning to stderr); it only errors when *every* shard fails.

Router configuration also supports `SHARD_HOSTS`, `GITHUB_TOKEN`, `WATCH_REPOS`, and `WATCH_ORGS`. Every shard starts a required local Zoekt process and maintains a persistent Zoekt index for the repositories assigned to its shard. The zoekt-webserver is started in the background and its readiness does not block the shard from serving; until it is up (or for repositories not yet reindexed) searches fall back to scanning. Zoekt selects candidate files before `grepple` verifies matches and builds tree-sitter context. When Zoekt supplies the candidate set the shard searches exactly those files and skips the filesystem walk entirely; because Zoekt only indexes git-tracked files, the candidates are already inside the repo, outside `.git`, and not `.gitignore`d, so the per-file `.gitignore` lookup is skipped too (only a full-scan fallback walks the tree and applies ignore rules). When a search scopes to specific repositories (`--repo`), each shard first checks which of its own repositories match: if none do it returns immediately without touching Zoekt or the filesystem, and if some do, a `--files` listing walks only their directories (content searches still verify Zoekt candidates and post-filter by repo). Both literal and regular-expression queries are translated to Zoekt's `content:` field (a Go/RE2 regexp that Zoekt optimizes into a substring search when it has no operators), so regex searches are pre-filtered through the trigram index rather than scanned in full. Files that Zoekt cannot represent without omissions are tracked as scanner-only supplements. A full filesystem scan is only used as a fallback: for queries shorter than three characters, for invalid regular expressions, or when Zoekt is unavailable or crashes. An incomplete (result-cap) Zoekt response is not a fallback trigger — grepple returns the valid partial candidates and flags the response `truncated` so the caller can narrow. Shard `/health` is a cheap, lock-free liveness signal (it reports `zoekt` process liveness and `repos`, and does not contend on the indexing mutex). Options `--zoekt-index`, `--zoekt-port`, and `--zoekt-bin` control the required process; Zoekt cannot be disabled.

## Container image and ECR

The `Dockerfile` builds a single image that ships the `grepple`, `shard`, and `router` binaries alongside the embedded `zoekt-git-index` and `zoekt-webserver` executables; the entrypoint arguments select which command runs.

The image is published to the Orion management-account registry `840707826088.dkr.ecr.eu-west-1.amazonaws.com/grepple` (region `eu-west-1`).

- `k8s/helm/values-aws-resources.yaml` declares the `grepple` ECR repository.
- `.github/workflows/aws-resources.yaml` creates it through helm-starters' `_reusable-deploy-centralized-aws-resources` workflow (deployed to `management-eks`). Merge changes to `main` to apply them.
- `.github/workflows/build_deploy.yml` builds the image and pushes `grepple:${GITHUB_SHA}` on every build, plus `grepple:latest` on `main`. Self-hosted runners authenticate to ECR through their node instance-profile role, so no explicit `docker login` step is required.

Create the ECR repository before the first image push: land `values-aws-resources.yaml` and `aws-resources.yaml` on `main` (or run the AWS Resources workflow manually), then let `build_deploy.yml` push images.

The same pipeline runs a `build_cli` job that compiles the `grepple` CLI natively on `generic-l` (amd64) and `generic-l-arm` (arm64) runners — native builds are used because the CLI links CGO and native tree-sitter grammars — and uploads the static binaries as run artifacts `grepple-linux-amd64` and `grepple-linux-arm64`.

## Playground deployment

`k8s/helm/values-playground-eks.yaml` deploys grepple to `playground-eks` through helm-starters' `app` chart: the shards run as a `StatefulSet` (`grepple-shard`) with a persistent `/data` volume for checkouts and the Zoekt index, exposed through the headless service `grepple-shard-headless`; the router runs as a stateless `Deployment` (`grepple-router`) that lists the shards' stable pod DNS names in `SHARD_HOSTS` and is reachable internally at `grepple.playground.ezprk.fun`.

Deployment is part of the `build_deploy.yml` pipeline: `build` → `image_check` (Trivy scan + signing) → `deploy_playground`, which deploys the freshly built `${GITHUB_SHA}` image. Pushes to `main` deploy for real; pull requests run the deploy step as a dry-run to validate the manifests. The pipeline can also be triggered manually from the Actions tab.

The values file declares placeholder `sealedSecrets`: `grepple-github-token` (key `token`, required by the router for GitHub API access and private clones) and `grepple-webhook-secret` (key `secret`, optional). Replace each `REPLACE_WITH_SEALED_*` value with a real sealed value before deploying, for example:

```bash
echo -n "$GITHUB_TOKEN" | kubeseal --raw --namespace <ns> --name grepple-github-token --scope strict
```

The router pod waits until `grepple-github-token` decrypts successfully; the webhook secret is referenced with `optional: true`, so the router still starts without it.

## Security scanning

CI uses reusable security workflows:

- `.github/workflows/codeql-scan.yaml` runs CodeQL over the Go code (`language: go`) on push, on demand, and monthly.
- `.github/workflows/trivy-scan-conf.yaml` runs a Trivy misconfiguration scan of the repository (fails on `CRITICAL`).
- `.github/workflows/build_deploy.yml` includes an `image_check` job that runs a Trivy image scan (`vuln,misconfig,secret`, `CRITICAL,HIGH`) and signs the pushed image; a signed image is required to deploy. Suppress accepted findings in `.trivyignore`.

CodeQL uses autobuild; because grepple builds with CGO and native tree-sitter grammars, switch to the build-command CodeQL variant if autobuild cannot compile the module.

Known, unfixable Trivy findings that are intentionally left undismissed (not suppressed in `.trivyignore`) are documented in [`docs/security-exceptions.md`](docs/security-exceptions.md).
