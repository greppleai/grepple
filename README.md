# grepple

Structure-aware grep for agents, implemented in Go. Search works across text files, while supported programming-language files receive compact tree-sitter structural context. Results are returned in deterministic path order (there is no relevance ranking) so you refine by narrowing with filters rather than trusting a score.

## Project layout

```text
.grepple/          Canonical machine-generated architecture artifacts
api/               Dependency-free HTTP request and response contracts
cmd/grepple/       Minimal executable entry point
docs/              User and file-type documentation
extract/           Tree-sitter Mermaid extraction, validation, and canonical architecture bundles
hooks/             Project-local Pi hooks and architecture tooling
internal/cli/      CLI workflows and output rendering
parser/            Language detection, tree-sitter parsing, segments, and outlines
search/            Discovery, matching, filtering, paging, and result construction
```

The private distributed service, repository registry, router, shard, and Zoekt integration live in the sibling `grepple-backend` repository. This public module never imports the backend.

All Go source is formatted with `gofmt`. The project requires Go 1.25. Tree-sitter uses CGO, so builds also require a C compiler.

## Build and install

Prebuilt static `grepple` CLI binaries for Linux (amd64 and arm64, with sha256 checksums) are attached to every GitHub release. Releases and the changelog are automated with [release-please](https://github.com/googleapis/release-please); use [conventional commits](https://www.conventionalcommits.org/) (e.g. `feat:`, `fix:`) so changes are picked up for the next release.

To build from source:

```bash
go test ./...
make build
make install
```

`grepple --version` (also `grepple version`) prints the release/source version, full commit revision, source commit timestamp, Go toolchain, and target platform. Make builds inject `git describe`, `git rev-parse HEAD`, and the commit timestamp; release builders can override `VERSION`, `COMMIT`, and `BUILD_DATE`. The source timestamp is used instead of the wall-clock build time to preserve reproducibility.

Linting uses [revive](https://github.com/mgechev/revive) with the pinned rule set in `revive.toml` (the documented default rules, made explicit, plus `cognitive-complexity` capped at 15):
`make lint` runs revive plus the standalone pi hook module's lint and tests. Pi executes a cached `hooks/bin/pi-hook` binary; the lightweight Make targets in `.pi/settings.json` rebuild it only when its Go sources or module files change. The Stop hook runs
on every agent Stop: it auto-fixes formatting with `gofmt -w`, then feeds one revive rule-group (worst file first, one
rule at a time) back to the agent with a remediation guide from `hooks/guides/<rule>.md` until the tree is clean.
Project analyzers in `hooks/internal/pihooks/` add repo-specific checks that revive cannot express. The current
`same-file-struct-methods` analyzer uses tree-sitter Go syntax trees and requires every method of a struct to live in the file that declares the type.
The PreToolUse grep guard is also implemented in the standalone module and uses the tree-sitter Bash grammar to inspect actual command invocations.

Mermaid extraction and validation live in the main `extract` package and are exposed through `grepple extract`; the hooks module imports that package for automatic Stop validation rather than maintaining a second analyzer. This repository exports canonical architecture artifacts to root [`.grepple/`](.grepple/): `make schema-generate` rewrites them and `make schema-check` validates them through the Grepple binary. Package bundles contain `manifest.json`, `overview.mmd`, and `structure.mmd`; workspace bundles contain `manifest.json` and `overview.mmd`. The Stop hook validates legacy `*.class.mmd`, `*.structure.mmd`, and `*.flow.mmd` schemas plus canonical package/workspace bundles. See [`hooks/README.md`](hooks/README.md) for automatic validation details.

The `grepple` application binary is built into `bin/grepple`.

The default Linux build links the CGO tree-sitter runtime and all grammars into self-contained static binaries. Command-line parsing uses [`go-arg`](https://github.com/alexflint/go-arg).

## Search

```bash
grepple "console\\.log" "src/*.ts"
grepple -F "console.log" src
grepple -E "console\\.(log|warn)" -r src
grepple --files "config/*.yaml" "config/*.yml"
grepple --count "httpRoute"
```

Supported search options:

- `-E`/`--regex` (JavaScript regular expressions; this is already the default), `-F`/`--fixed-strings`, `-i`/`--ignore-case`
- `-r`/`--recursive` is accepted as a compatibility no-op because directory searches are recursive by default
- `-n`/`--line-number`, `--line-only`, `-C`/`--context N`
- `-l`/`--files` (list files whose **path** matches the glob), `--files-with-matches` (list paths of files whose **contents** match, like `grep -l`), `-c`/`--count`
- `--json`, `--json-matches`
- `--repo PATTERN` (repeatable)
- `--max-files N`, `--max-segments N`
- `--max-output-bytes N` caps human-readable output before common agent tool limits (default `40960`; `0` disables the cap). JSON output is never partially truncated.
- `--related` (experimental: show bounded project-local callees and callers for structurally supported source languages)
- `--at PATH:LINE` (retrieve the declaration containing an exact local location; related `PATH:START-END` ranges are accepted too)
- `--follow-related N` (expand up to two unique callees per level, depth 1-3; implies `--related`)
- `--skip N`, `--limit N` (page through results in deterministic path order: skip the first `N` files / return at most `N`; **`--limit` defaults to `20`**, use `--limit 0` for all). A server never returns more than **100 files per page** — `--limit 0` or a larger value gets the maximum page, and you page further with `--skip N`; local-only searches stay uncapped

### Grep migration and editing workflow

Grepple is not a complete GNU/BSD grep flag emulation layer. Patterns use JavaScript regular-expression syntax by default; `-E` is accepted as an explicit alias for that default, while `-r` is accepted because filesystem searches are already recursive. Use `-F` when the pattern must be literal. Other unsupported grep flags should be translated to Grepple's output model rather than copied mechanically.

For agent edits, use Grepple as the discovery step and the agent's Read tool as the edit-preparation step:

```text
grepple --line-only or structural search → Read the exact PATH:LINE range → Edit with hash anchors
```

Default structural output shows enclosing declarations and may collapse unrelated lines. If a segment limit omits matching lines, Grepple reports the omitted count and recommends `--line-only`. Broad human-readable output is bounded before common tool-result limits and ends with narrowing guidance; tighten the path/glob or use `--limit`, `--line-only`, `-l`, or `--count`.

### Source call navigation

For local searches, `--related` adds bounded navigation hints after each structural result. It supports Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell:

```bash
grepple --related -F "g.auditor.Record" examples/advanced-files
```

```text
Next points (code navigation):
  → g.auditor.Record → Auditor.Record  examples/advanced-files/gateway.go:12-12  call:32
  → g.deliver → Gateway.deliver  examples/advanced-files/gateway.go:18-18  call:35
```

Grepple indexes declarations only within the files selected by the search paths/globs and resolves names only within the same language. `→` marks callees and `←` marks potential callers. Functions, methods, and constructors are indexed across supported languages; Go additionally indexes interface methods and function-valued struct fields. Go and TypeScript navigation use explicit imports, direct parameter/method-receiver types, source-ordered top-level typed or constructor/composite-literal locals, and same-file typed member chains before terminal-name fallback. JSON confidence is `exact` for a qualified identity match, `import-resolved` when an explicit import identifies the target package/module, `context-resolved` when declaration kind, file locality, or receiver type safely narrows candidates, `unique-terminal` when only one declaration has the terminal name, and `candidate` when ambiguity remains. Text output gives candidates an explicit `--at PATH:LINE` suggestion. This is syntax-based navigation, not a type-checked call graph: nested lexical bindings, call-return inference, cross-file field chains, and embedded/promoted methods may remain candidates. Standard-library and external calls are omitted because they have no declaration in the selected files. Callees and callers are each capped at five points per declaration, with at most three candidates for one ambiguous call. Production callers are preferred over conventional test filenames.

Retrieve one declaration directly from a navigation location:

```bash
grepple --at search/result.go:32
```

Or explicitly spend more tokens to inline a bounded call chain:

```bash
grepple --follow-related 1 -F "attachRelated(out" search
```

Each level expands at most two resolved outgoing callees; callers and ambiguous candidates remain compact hints. Expansion depth is capped at three, cycles are not expanded again, and expanded declarations share a 400-line budget per root result. Every expansion remains available structurally in full `--json`. These experimental navigation modes currently support local default structural output and full JSON only.

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
grepple --outline search/search_engine.go     # one local file
grepple --outline "**/*.go"                   # every Go file
grepple --outline --json src/app.ts            # machine-readable {"files":[...]}
grepple --outline values.yaml                  # JSON/YAML: key tree, values omitted
grepple --outline --depth 2 large-config.json  # cap nesting (JSON/YAML only)
grepple get OWNER/REPO path/to/File.java -O    # a remote indexed file
```

Output is `START-END<TAB>KIND<TAB>NAME`, with members indented under their
container:

```
search/search_engine.go	go
229-234	struct	candidateScan
240-249	method	(candidateScan).scanAll
254-277	method	(candidateScan).scanWindowed
```

The tree-sitter languages (Go, JavaScript/TypeScript, Python, Java, Kotlin, C#,
C, C++, Rust, and Shell) get full symbol outlines; Markdown gets a heading outline (`h1`…`h6`, code-fence aware);
**JSON/YAML** get a key/type tree (`object`/`array`/`string`/`number`/`bool`/`null`)
with values omitted — arrays show a `[N]` length and recurse only into object/array
elements (scalar lists collapse), scalars are typed via the resolved YAML tag, and
**multi-document YAML** (`---`) is supported as `document [i]` nodes. Other file types
produce an empty outline. Go methods are shown at top level as `(*Type).Method`.
`--outline` composes with `--repo`/`--limit`/`--skip` (and `--depth N` for JSON/YAML,
0 = unlimited) and cannot be combined with `--count` or `--files-with-matches`.

## Architecture and flow extraction

`grepple extract` creates deterministic, self-validated Mermaid navigation maps from Tree-sitter source analysis. Focused structure and flow extraction support Go, TypeScript, and TSX; canonical package and workspace bundles remain Go-specific. Generic callable declarations and calls use the normalized `parser.NavigationGraph` shared with `--at` and `--related`. Extraction enriches graph declarations and edges with stable identities, package/module scope, receiver/container context, imports, resolved targets, and confidence; focused flow generation and validation consume those enriched edges directly.

```bash
grepple extract structure internal/cli
grepple extract structure internal/cli --entry cliOptions --source .
grepple extract flow internal/cli --entry runSearch --depth 2
grepple extract flow --at internal/cli/local.go:13 --source .
grepple extract structure --at web/store.ts:8 --source web
grepple extract structure api --bundle --output .grepple/api.package
grepple extract structure . --workspace --output .grepple/project.workspace
```

`structure` without an entry generates the complete Go package class diagram for the selected file or directory. `--entry` or `--at PATH:LINE` produces a bounded, language-adapted type structure. `flow` requires one of those selectors and follows statically resolved outgoing calls. `--source` can be repeated to define the analysis roots; `--depth`, `--max-nodes`, and `--output` control projection and writing. A flow that reaches `--max-nodes` remains valid and deterministic, and includes `%% grepple:truncated max-nodes N` instead of failing after useful nodes have already been selected.

Every generated type/function node includes its exact `PATH:START-END` definition range; type notes also list exact member ranges. Flow nodes display their definition range directly. Generated diagrams are checked against the same source analysis before being returned.

Validation is available through `grepple extract check structure|flow|package|workspace`. `extract.SupportedLanguages` and `extract.LanguageForPath` expose adapter metadata. To add a language, implement one adapter with its Tree-sitter analyzer and structure/flow projections, then add it to `registeredLanguages`; discovery and orchestration require no language-specific branches.

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

## Remote service

The distributed router, shards, repository synchronization, Zoekt integration, and deployment assets live in the private sibling `grepple-backend` repository. This public repository contains only the CLI and shared search contracts.

Search remains local-first. It contacts a remote service only when `--remote` (`-R`) or `--server` is supplied:

```bash
grepple "useEffect" src                         # local only
grepple --remote --repo owner/repo "useEffect"  # local plus configured service
grepple --server https://grepple.example.com --repo owner/repo "useEffect"
grepple repos                                    # indexed repository catalog
grepple get owner/repo path/to/file --lines 20:50
grepple tree owner/repo src --depth 3
```

Remote server URLs are resolved from `--server`, `GREPPLE_SERVER`, `./grepple.json`, `~/.grepple/config.json`, and finally `http://127.0.0.1:8787`.

### Login

A compatible service can advertise GitHub OAuth device-flow configuration. Authenticate and persist the resulting token with:

```bash
grepple login --url https://grepple.example.com
grepple logout
```

Remote search, `get`, `tree`, and rule commands send the stored bearer token. `GREPPLE_TOKEN` can provide an externally managed token instead.
