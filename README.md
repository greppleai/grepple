# grepple

Structure-aware grep for agents, implemented in Go. Search works across text files, while supported programming-language files receive compact tree-sitter structural context. Results are returned in deterministic path order (there is no relevance ranking) so you refine by narrowing with filters rather than trusting a score.

## Project layout

```text
.grepple/          Canonical machine-generated architecture artifacts
api/               Dependency-free HTTP request and response contracts
cmd/grepple/       Minimal executable entry point
dependency/        Package-manager resolver interfaces, manifest/lock adapters, and source identity
docs/              User and file-type documentation
extract/           Tree-sitter Mermaid extraction, validation, and canonical architecture bundles
gritql/            Native, bounded multi-language Tree-sitter structural detection kernel
gritqlapi/         Adapters from structural findings to dependency-free API DTOs
hooks/             Project-local Pi hooks and architecture tooling
internal/aiprovider/ Provider-neutral AI authentication, credential storage, and model adapters
internal/cli/      CLI workflows and output rendering
parser/            Language detection, tree-sitter parsing, segments, and outlines
rulespec/          Shared validation for text and structural saved rules
search/            Discovery, matching, filtering, paging, and result construction
```

The private distributed service, repository registry, router, shard, and Zoekt integration live in the sibling `grepple-backend` repository. This public module never imports the backend.

All Go source is formatted with `gofmt`. The project requires Go 1.25. Tree-sitter uses CGO, so builds also require a C compiler.

## Build and install

Prebuilt `grepple` CLI binaries are attached to every GitHub release for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64, with SHA-256 checksums and build-provenance attestations when GitHub enables them for the repository. Google's [Release Please](https://github.com/googleapis/release-please) manages conventional-commit release pull requests and semantic tags starting at `v0.0.1`; native CGO runners build and smoke-test every target. See [Releases](docs/releases.md) for the publishing and validation workflow.

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
`same-file-struct-methods` is now declared in `.grepple/hooks/same-file-struct-methods.yaml`: GritQL node patterns capture Go struct types and method receivers, and a reusable cross-file relation joins them by directory, package, and normalized type. The standalone Pi Stop module only adapts the resulting findings to its revive-shaped feedback protocol.
The PreToolUse grep guard is also implemented in the standalone module and uses the tree-sitter Bash grammar to inspect actual command invocations.

Mermaid extraction and validation remain available in the `extract` package for the Stop hook, but are no longer CLI commands. Use `grepple architecture directory` for source-linked directory orientation and `grepple graph callers|callees` for focused call navigation. `make schema-generate` and `make schema-check` validate parser-generated language metadata. The Stop hook validates `*.class.mmd`, `*.structure.mmd`, and `*.flow.mmd` schemas.

Public parser consumers should treat `Document` as the owning parse boundary, `Node` as a document-tied handle, `DocumentView`/`ViewNode` as callback-scoped lock-free traversal, and `SyntaxNode` as the persistent immutable snapshot. See [Parser syntax lifecycle](docs/parser-syntax-lifecycle.md) for ownership, locking, invalidation, and snapshot guidance.

The `grepple` application binary and optional user-level `greppled` report cache are built into `bin/`. Start one `greppled` from any directory; from any repository, use `grepple --daemon architecture directory .` or a focused graph command such as `grepple --daemon graph callers --symbol Run .` (`resolve` and `callees` are also supported). On a miss the CLI builds locally in its own working directory and publishes the completed report; later matching requests use the shared worker. No repositories are eagerly scanned. The worker stays in the foreground until interrupted; an unavailable or incompatible worker silently falls back to direct local analysis. Remote requests do not use it. See [architecture performance benchmarks](docs/architecture-performance-benchmarks.md) for limits and measurements.

The default Linux build links the CGO tree-sitter runtime and all grammars into self-contained static binaries. Command-line parsing uses [`go-arg`](https://github.com/alexflint/go-arg).

## Agent utility metrics

Grepple can analyze externally generated `grepple-metrics-event-v1` JSONL without collecting or discovering evidence itself. Every run identifies its producing coding agent, reports are always grouped by agent, and comparisons consume two previously generated JSON reports:

```bash
grepple metrics report --input ./without-grepple.jsonl --format json > baseline.json
grepple metrics report --input ./with-grepple.jsonl --format json > target.json
grepple metrics compare --baseline baseline.json --target target.json
```

Journal and report inputs are always explicit. This allows two runs from the same coding agent—for example, one without Grepple and one with Grepple—to be compared without reparsing their JSONL during comparison. See [Agent utility metrics](docs/agent-utility-metrics.md) for the strict event and report contracts, privacy rules, bounds, and output formats.

## Search

```bash
grepple "console\\.log" "src/*.ts"
grepple -F "console.log" src
grepple -E "console\\.(log|warn)" -r src
grepple --files "config/*.yaml" "config/*.yml"
grepple --count "httpRoute"
grepple --count-summary "httpRoute" .
```

Run `grepple examples` for copyable task workflows covering workspace orientation, exact retrieval, edit anchors, caller impact, boundary review, native structural audits, focused diagrams, and canonical architecture checks. Pass a task name such as `grepple examples impact` for one concise workflow. See [`docs/output-contracts.md`](docs/output-contracts.md) for the cross-command human/JSON/Mermaid contracts, exact limit units, delivery spilling, and local-versus-remote availability.

Supported search options:

- `-E`/`--regex` (JavaScript regular expressions; this is already the default), `-F`/`--fixed-strings`, `-i`/`--ignore-case`
- `-r`/`--recursive` is accepted as a compatibility no-op because directory searches are recursive by default
- `-n`/`--line-number`, `--line-only`, `-C`/`--context N`
- `-l`/`--files` (list files whose **path** matches the glob), `--files-with-matches` (list paths of files whose **contents** match, like `grep -l`), `-c`/`--count` (per-file counts in the selected page), `--count-summary` (complete matched-file/matching-line totals independent of `--skip`, `--limit`, and `--max-files`). `--count-by-repo` retains the grouped compatibility view.
- `--json`, `--json-matches`
- `--repo PATTERN` (repeatable)
- `--max-files N`
- `--max-output-bytes N` caps human-readable output before common agent tool limits (default `16384`; `0` disables the cap). JSON output is never partially truncated.
- Eligible local source results always emit hashline anchors as `HASH│LINE│content`; named compatibility providers are selected only through user-owned settings
- `--related` (explicitly request the default bounded repository-local type/caller/callee navigation)
- `--no-related` (disable automatic navigation for structural search)
- `--follow-related N` (1-3 outgoing call hops, up to two followed callees per level; structural search also shows immediate callers of the matched declaration, but never follows them)
- `--skip N`, `--limit N` (page through results in deterministic order: skip the first `N` files / return at most `N`; **`--limit` defaults to `20`**, use `--limit 0` for all). A server never returns more than **100 files per page** — `--limit 0` or a larger value gets the maximum page, and you page further with `--skip N`; local-only searches stay uncapped
- `--sort path|matches` keeps repository/path order by default or opts into matching-line count descending with repository/path tie-breakers. Match-count sorting scans the full selected candidate universe before paging.

### Grep migration and editing workflow

Grepple is not a complete GNU/BSD grep flag emulation layer. Patterns use JavaScript regular-expression syntax by default; `-E` is accepted as an explicit alias for that default, while `-r` is accepted because filesystem searches are already recursive. Use `-F` when the pattern must be literal. Other unsupported grep flags should be translated to Grepple's output model rather than copied mechanically.

For agent edits, Grepple can either locate lines for a normal Read step or emit harness-compatible edit anchors through a user-configured provider:

```text
grepple --line-only or structural search → Read exact PATH:LINE or PATH:START-END → Edit with hash anchors
grepple search (with anchors enabled in settings) → Edit directly with emitted HASH│LINE│content rows
```

`HASH` is the edit key and `LINE` is the 1-indexed orientation/fallback location used by Read and `--at`. `--at PATH:LINE` retrieves the enclosing declaration, while an explicit `--at PATH:START-END` retrieves that exact range; `--line-only` selects line-oriented output. Eligible local structural, contextual, and line-only searches always use anchors, including bounded `-C`/`-A`/`-B` context and exact range retrieval. Grepple's built-in `hashline-v1` implementation is the default. `grepple write` auto-detects either one strict `grepple-write-v1` JSON request or a literal `::grepple file` heredoc transaction on stdin, and prevalidates anchored edits plus explicit transactional creates and digest-guarded deletes across confined paths. Heredoc transactions provide the same multi-file features without JSON or shell escaping and support caller-selected `--end-marker` terminators for directive-looking source. For one edit, `grepple write edit --path PATH --start HASH [--end HASH]` reads literal replacement text from stdin or `--content-file`. Write responses are edit-ready `HASH│LINE│content` with freshly recomputed anchors; use `--json` for structured automation, and `--dry-run` for deterministic unified diffs plus predicted anchors. Named provider commands remain available for editor compatibility, execute without a shell, and use the versioned batch protocol. `grepple anchors setup` and `anchors doctor` manage those optional providers. See [Transactional hashline writes](docs/write.md) and [Edit-anchor providers](docs/anchor-providers.md).

Default structural output shows complete enclosing functions and methods, retaining only required parent wrappers and excluding proximity-selected imports, neighboring declarations, and nonmatching siblings. Plain `--line-only` output remains one row per match, but parser-backed source lines that begin a multi-line construct use `PATH:START-END:text` (for example a function or `if` statement) so the next Read can retrieve its exact extent; ordinary and unparsed lines retain `PATH:LINE:text`. Add `--enclosing` to annotate a body match with its nearest multi-line syntax scope as `PATH:MATCH@START-END:text`, such as `worker.go:122@121-123: save()`. Anchor rows preserve the exact `HASH│LINE│content` editing contract, so `--enclosing` automatically avoids default anchors and rejects explicitly requested anchors. Broad human-readable output is bounded before common tool-result limits and ends with narrowing guidance; tighten the path/glob or use `--limit`, `--line-only`, bounded context, `-l`, or `--count`.

### Source call navigation

Structural search automatically adds bounded navigation hints for local or remotely indexed repositories; use `--no-related` to suppress navigation or `--follow-related N` to select 1-3 call hops. It retains immediate caller links for the matched declaration but expands only outgoing callees (A → B → C, not other callers of B). Depth 1 shows direct calls; depth 2 adds their callees. Human output keeps navigation as a compact call tree with exact `PATH:LINE` selectors instead of rendering source bodies. Exact `--at PATH:LINE` retrieval shows only outgoing calls, including at the root. It supports Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell:

```bash
grepple --related -F "g.auditor.Record" examples/advanced-files
grepple --server http://localhost:8080 --repo gofiber/fiber --related -F 'func New(config'

```text
Next points (code navigation):
  → g.auditor.Record → Auditor.Record  examples/advanced-files/gateway.go:12-12  call:32
  → g.deliver → Gateway.deliver  examples/advanced-files/gateway.go:18-18  call:35
```

Grepple searches only files selected by the requested paths/globs, while local navigation discovers the nearest repository/workspace root from `grepple.json`, `.git`, or `go.work` and resolves against that complete source universe; an explicit root remains authoritative. This keeps text matching narrow while making sibling packages and manifest-evidenced sibling modules available. Indexed remote navigation likewise uses the complete selected repository universe. `→` marks callees and source-declared parameter, receiver, local, or result types, while `←` marks potential callers. Human caller/callee output is a compact location tree; complete related source remains available through its `--at PATH:LINE` selector or full JSON. Only type points attached directly to the matched declarations are expanded, and human output deduplicates each referenced type by source range into one final **Related type definitions** appendix with editable `HASH│LINE│content` rows. Types belonging only to secondary callers or callees are not expanded. Functions, methods, constructors, and source-declared types are indexed across supported languages. Go additionally indexes interface methods, function-valued struct fields, cross-file typed fields, and embedded/promoted methods. TypeScript/TSX additionally use inheritance, named/default aliases, barrel re-exports, cross-file member chains, and nearest-`tsconfig.json` path aliases. Nested bindings remain scoped to their branch or block. JSON confidence is `exact` for a qualified identity match, `import-resolved` when an explicit import identifies the target package/module, `context-resolved` when declaration kind, file locality, receiver type, inheritance, or promotion safely narrows candidates, `unique-terminal` when only one declaration has the terminal name, and `candidate` when ambiguity remains. Text output gives candidates an explicit `--at PATH:LINE` suggestion. This remains syntax-based navigation, not compiler dispatch: interface implementations, overloads, conflicting promotions, malformed configuration, and unresolved aliases remain explicit candidates. Standard-library and unavailable external calls or types have no local target. Callees, callers, and type declarations are each capped at five points per declaration, and production callers are preferred over conventional test filenames. When a cap omits evidence, text and JSON report directional omission counts; caller/callee omissions point to the complete graph.

External navigation preserves unresolved imported call and type evidence, qualifies it from exact local dependency manifests and locks, then asks every shard's prebuilt navigation artifact index for the matching indexed version. Package-manager semantics live behind `dependency.Resolver`: each Go module, npm, Cargo, or Maven adapter owns project selection, manifest and lock interpretation, import matching, source identity, and artifact-module discovery, while navigation only applies normalized evidence. This keeps future pnpm, Bun, alternate Maven tooling, or authenticated registry policies out of the generic navigation coordinator. Go uses `go.mod`, `go.sum`, versioned replacements, and root `go.work`; Node uses direct `package.json` dependencies plus `package-lock.json` versions 1–3; Rust uses registry dependencies from `Cargo.toml` plus `Cargo.lock`; and Java/Kotlin use exact direct Maven `pom.xml` coordinates. JVM references carry every exact declared coordinate because source package names do not prove Maven ownership; indexed source declarations perform the final disambiguation. A unique declaration is returned as `dependency-resolved` with ecosystem, module version, integrity when available, repository selector, commit, artifact digest, source range, and source preview. Wrong versions, local/path dependencies, missing artifacts, and ambiguous declarations remain explicit rather than falling back to terminal-name guesses. Shards build deterministic packed-protobuf graph artifacts during repository ingestion; authenticated clients can fetch the immutable current artifact from `/public/navigation/artifact?repo=OWNER/REPO[@REF]`.

Retrieve one declaration directly from a navigation location:

```bash
grepple --at search/result.go:32
grepple --server http://localhost:8080 --repo gofiber/fiber --at app.go:721
```

Increase the default depth when a larger bounded call chain is useful:
```bash
  grepple --follow-related 2 -F "attachRelated(out" search
```

Each level expands at most two resolved callers and two resolved callees; ambiguous candidates remain compact hints. Expansion depth is capped at three, cycles are not expanded again, and expanded declarations share a 400-line budget per root result. Every expansion remains available structurally in full `--json`. Navigation defaults apply to local and remote structural output and full JSON, while count, file-list, line-only, context, and other compact modes remain navigation-free. Remote `--at` requires exactly one `--repo`, and remote navigation deliberately scans the complete selected repository source universe rather than a text-index candidate subset so declarations in non-matching files remain available.

File patterns use Go's `filepath.Glob` syntax, extended with `**` to match across directory boundaries (for example `**/*.yaml` or `charts/**/values.yaml`). Omit globs to search recursively from the working directory; a matched directory is also searched recursively. `.git` directories and repository `.gitignore` entries are excluded. Shard searches are confined to the served repository root, so client-supplied globs and paths cannot escape it. Globs compose with every output mode, including `-c`/`--count`. When a Zoekt index is available the globs are translated into a `file:` atom and pushed down to the index (a deliberate superset — the shard still applies the exact glob matcher to what the index returns), so the index pre-filters by path instead of shipping every content match for the shard to discard.

**Pipes work like `grep`/`rg`:** when standard input is piped (or redirected) and no path/glob argument is given, grepple searches the stream instead of the filesystem — `cat build.log | grepple "ERROR"` or `go test ./... | grepple -F "FAIL"`. The stream is reported under the virtual path `<stdin>` in every output mode (`--json`, `-c`, `--files-with-matches`, `--line-only`, default segments). It uses the plain-text fallback (no filename, so no tree-sitter structure), NUL-containing input is treated as binary and skipped, and no match exits 1 as usual. `--files` and `--outline` always operate on the filesystem, and passing any path/glob selects the filesystem over stdin.

Default structural search classifies every returned file as `structured`, `recovered`, `plain`, `unsupported`, or `failed`. Human output emits an incomplete source-analysis summary when any result is recovered, unsupported, or failed; intentional plain-text output remains quiet. Complete JSON includes each file's `structureStatus` and aggregate `sourceAnalysis`. Incomplete parsing is never silently presented as fully parser-backed context. [Search and segment ranking](docs/search-ranking.md) documents path and opt-in match-count file order plus complete matched-scope selection.

Complete search, graph, boundary, and CLI GritQL JSON also share a [result metadata](docs/result-metadata.md) envelope covering normalized scope, paging/completeness, source and byte caps, known omission totals, stable diagnostics, and a copyable continuation command when one is available. Unknown totals are omitted rather than estimated.
## Native structural search

`grepple grit` runs the unified native, read-only `gritql-v1` structural-search engine over every Tree-sitter-backed Grepple language: Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell. The target language is declared in the query; no separate compatibility flag is required. The engine is separate from text and regex search and has no external runtime, subprocess, rewrite engine, or fallback interpreter. The supported detection subset includes snippets, metavariables, repeated-binding equality, `where`, `contains`, `within`, `and`, `or`, `not`, `maybe`, and RE2 constraints.

The [`gritql-metric-v1` engine](docs/gritql-compatibility.md#31-source-authored-scoped-metrics) computes bounded per-function scores from **source-authored GritQL metric rules**. This repository enables [McCabe](.grepple/hooks/go-mccabe.yaml) and [nested-loop](.grepple/hooks/go-nested-loops.yaml) warnings; run `grepple hook --id go-mccabe` or `grepple hook --id go-nested-loops`. The nested-loop score is a syntactic heuristic, **not a Big-O proof**. The [cognitive example](examples/go-cognitive.yaml) is still disabled: [Revive parity measurements](docs/gritql-metric-parity.md) show remaining differences, so Revive remains authoritative.

```bash
# Quote inline queries so the shell does not expand metavariables.
grepple grit $'language go\n`exec.Command($args)`' '**/*.go'

# Every Tree-sitter-backed language uses the same command and compatibility contract.
grepple grit $'language javascript\n`fetch($url)`' '**/*.js' '**/*.jsx'
grepple grit $'language typescript\n`fetch($url)`' '**/*.ts'
grepple grit $'language tsx\n`<Button value={$value} />`' '**/*.tsx'
grepple grit $'language python\n`subprocess.run($args)`' '**/*.py'
grepple grit $'language rust\n`Command::new($program)`' '**/*.rs'

# Multiline query file and complete JSON output.
grepple grit --query-file /tmp/exec-command.grit --json '**/*.go'

# Compile only: explain target grammar wrappers, features, and metavariable roles.
grepple grit explain --json --query-file /tmp/exec-command.grit

# Merge local findings with bounded pages from a compatible router.
grepple grit --remote --repo 'acme/*' --query-file /tmp/exec-command.grit '**/*.go'
```

Findings contain exact half-open byte ranges, one-based Unicode-scalar positions, matched text, and sorted metavariable bindings. Local and remote findings are normalized, sorted, exactly deduplicated, and paged once as a combined result. Local scans have no implicit wall-clock timeout; `--max-file-time-ms` and `--timeout-ms` opt into deadlines. Count, size, memory, and cancellation bounds remain active. Unsupported target languages, rewrites, and external functions fail closed. `grit explain` compiles without reading source files and emits `grepple-grit-explain-v1`: target language and compatibility, grammar identity, used features, every grammar-valid wrapper/context interpretation, named metavariable occurrence counts, node/list binding kinds, wrapper fields, constraint roles, and bounded compile diagnostics.

Remote structural search requires a backend implementing `POST /public/grit`. See [`docs/gritql-compatibility.md`](docs/gritql-compatibility.md) for the exact closed syntax, evaluation rules, diagnostics, limits, security guarantees, conformance fixtures, and benchmark gates.

### Local hooks

`grepple hook` loads `*.yaml` from the nearest ancestor's `.grepple/hooks/` directory. File-local `gritql-v1` checks default to staged, unstaged, and untracked Git files; deleted files and symlinks are skipped. Selecting a `gritql-relational-v1` rule automatically uses the complete repository snapshot, including unchanged files needed for cross-file joins, even without `--all`. From a nested directory, paths and hooks remain relative to the repository root. `--all` scans all selected sources, including outside Git repositories. Repeat `--id ID` to run only named hooks; unknown IDs and invalid selected configs fail instead of silently succeeding. These checks are read-only; this CLI command does not run Pi's grep guard, format code, invoke Revive, or emit Pi hook protocol output.

```bash
grepple hook                                      # all configured checks; relations force full scope
grepple hook --id go-empty-if --id go-direct-dot-import # changed-file checks only
grepple hook --id same-file-struct-methods        # full cross-file Go rule
grepple hook --all --json                         # full repository audit
```
Each config declares `version: 1`, an ID matching its filename, `event: Stop`, a repository-relative `include` list (optional `exclude` globs), `severity: warning|error`, and a message. File-local rules use `engine: gritql-v1` and one GritQL `query`; relational rules use `engine: gritql-relational-v1` with `relation.left_query`, optional `right_query` and `partition_query`, captured keys, `scope: directory|repository`, and optional `unique_left`. Message placeholders include `{{key}}`, `{{left.basename}}`, and named captures such as `{{right.method}}`. `--workers N` scans up to four files in parallel (default four; `--workers 1` runs serially). Applicable queries share a parse within each file in their scan group, and findings are ordered deterministically. File-local results are cached by source SHA-256, selected rules, and executable identity. Relational results use a complete-snapshot cache with the same integrity checks; changing an eligible source invalidates that relation's cached result. Failed or truncated scans are never cached as clean. Findings include source location, ID, severity, and message; human output ends with a scope/count summary. No findings exit 0, any findings exit 1 (including warnings); invalid config, Git discovery failures, source diagnostics, and scan truncation fail with an error rather than reporting a clean audit. The legacy `go-empty-if` ID also detects empty loops, switches, and standalone blocks; `parent kind("function")` excludes empty function bodies without enumerating declaration signatures. Empty selects and condition-call loops remain exempt. The demo direct-dot-import hook matches only ungrouped imports. Neither demo claims full Revive-rule parity.

Relational rules can also use `relation.mode: unmatched_left` to report source-authored declarations without a matching right-side reference anywhere in their selected scope. The [installed Go uncalled-method candidate hook](.grepple/hooks/go-uncalled-methods.yaml) can be run with `grepple hook --id go-uncalled-methods --all`; its [reusable example](examples/go-uncalled-methods.yaml) can be copied to other repositories. It excludes `*_test.go`, so a method referenced only by Go tests is reported, while methods declared in tests are outside the rule. Name-based absence is a review lead, not proof of unreachable code.

The [private Go type](.grepple/hooks/go-dead-private-types.yaml), [exported internal type](.grepple/hooks/go-dead-internal-types.yaml), and [exported internal method](.grepple/hooks/go-dead-internal-methods.yaml) rules use the same source-authored relation engine. `node_pattern() as $name` captures a matched leaf node; `relation.left_include` limits declarations to `internal/` while references still come from all included Go sources. Large reference sets can opt into a bounded `relation.max_findings` (up to 100,000). All three exclude `_test.go` and report review candidates, not safe-to-delete declarations.

Each source-authored metric rule uses `engine: gritql-metric-v1` with a GritQL metric document in `query` and a strictly greater-than threshold declared as `above` inside that document. Metric rules use changed-file discovery and content-validated caching; metric file scanning is currently serial and does not use `--workers`.

The Go rule’s control-flow branch uses `` `{ $body }` where { $body <: empty } `` to test structural emptiness. Separate switch patterns cover Go’s different switch-body syntax.

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
grepple --outline --kind types parser/language.go  # declarations only (struct/interface/type/etc.)
grepple --outline --kind functions --kind variables src/  # combine categories
grepple --outline "**/*.go"                   # every Go file
grepple --outline --json src/app.ts            # machine-readable {"files":[...]}
grepple --outline values.yaml                  # JSON/YAML: key tree, values omitted
grepple --outline --depth 2 large-config.json  # cap nesting (JSON/YAML only)
grepple get OWNER/REPO path/to/File.java -O    # a remote indexed file
grepple get OWNER/REPO path/to/File.java -O --kind functions  # methods/constructors
```

Output is `START-END<TAB>KIND<TAB>NAME`, with members indented under their
container:

```
search/search_engine.go	go
229-234	struct	candidateScan
240-249	method	(candidateScan).scanAll
254-277	method	(candidateScan).scanWindowed
```

`--kind types|functions|variables` filters code declarations in both local and
`get --outline` output (repeat the flag or comma-separate names to combine them).
Types include classes, structs, interfaces, enums, traits and aliases; functions
include methods and constructors; variables include constants, fields and
properties. Matching members inside an unselected container are still shown,
without that container. With a filter, human output never falls back to raw
source, even for short files. JSON/YAML key trees and Markdown headings do not
belong to these declaration categories.

The tree-sitter languages (Go, JavaScript/TypeScript, Python, Java, Kotlin, C#,
C, C++, Rust, and Shell) get full symbol outlines; Markdown gets a heading outline (`h1`…`h6`, code-fence aware);
**JSON/YAML** get a key/type tree (`object`/`array`/`string`/`number`/`bool`/`null`)
with values omitted — arrays show a `[N]` length and recurse only into object/array
elements (scalar lists collapse), scalars are typed via the resolved YAML tag, and
**multi-document YAML** (`---`) is supported as `document [i]` nodes. Other file types
produce an empty outline. Go methods are shown at top level as `(*Type).Method`.
`--outline` composes with `--repo`/`--limit`/`--skip` (and `--depth N` for JSON/YAML,
0 = unlimited) and cannot be combined with `--count` or `--files-with-matches`.

## Normalized navigation graph
Focused `grepple graph callers|callees --json [PATH...]` returns the relevant subgraph of the deterministic `grepple-navigation-graph-v7` parser facts shared with `--at`, `--related`, and directory architecture. Callable nodes include stable IDs, language, repository-relative paths, declaration line ranges, package/module/container context, source-written signatures, adapter-evidenced entrypoint roles, and visibility. Calls include caller IDs, resolved target IDs, deterministic candidate target IDs, line locations, and resolution confidence. Resolution remains syntax-based and preserves ambiguity rather than claiming runtime dispatch. Reports include discovered, selected, parsed, skipped, failed, and recovery-parse source counts. JSON is never byte-truncated; `--max-files N` records deterministic source-file truncation separately.

Per-file parser facts are cached as deterministic path-neutral packed-protobuf entries under `~/.grepple/cache/<repository-id>/navigation/` by default. The v27 codec serializes the native graph directly through a shared string table and packed columns, validates a payload checksum, bounds entry and column sizes, and atomically writes immutable content-addressed `.pb` files. Cache failures remain non-authoritative, and cold/warm graph output is identical.

```bash
grepple graph callers --at internal/cli/extract.go:32 --depth 2 --language go .
grepple graph resolve --symbol runExtract ./internal/cli
grepple graph callees --symbol runExtract --depth 2 --json ./internal/cli
grepple graph callees --root-path internal/cli --depth 1 .
grepple graph callers --symbol runExtract --depth 2 ./internal/cli
grepple graph callers --server http://127.0.0.1:8080 --repo OWNER/REPO@tag~v1.2.3 --symbol Run
```

Focused caller/callee trees show available source-written callable signatures (parameters and explicit return syntax) by default; they do not infer runtime argument values or missing types. `graph resolve` lists ambiguous declaration matches with copyable selectors. Human output is bounded and includes per-edge confidence and incomplete/truncated-source warnings; `--max-output-bytes` never applies to JSON. Complete JSON retains candidate IDs and diagnostics. Human output omits calls without a repository-local target, and ambiguous candidates remain explicit rather than being guessed.

Graph queries, `--related`, and directory architecture share parser-owned per-file navigation facts under `~/.grepple/cache/<repository-id>/navigation/`. Entries are addressed by source content, language, grammar ABI/fingerprint, and a cache schema; path-neutral facts are instantiated with each command's requested path. Corrupt or unwritable entries are ignored, writes are atomic, and cold and warm reports are byte-identical. Set `GREPPLE_CACHE_DIR` to override the base cache location, or `GREPPLE_NAVIGATION_CACHE_DIR` to relocate navigation entries (an empty value disables them).

`graph resolve --symbol NAME` performs no traversal. It previews all exact-name matches, or terminal-name matches when there is no exact match, with full stable declaration IDs, exact `PATH:LINE` selectors, and copyable callers/callees commands. Repeatable language and visibility filters narrow alternatives before selection. Bounded human-readable output is the default; use `--json` for the complete `grepple-navigation-resolve-v1` projection when overloaded or cross-container names require machine-readable disambiguation.
`graph callers` and `callees` return deterministic subgraphs over repository-local navigation edges. Select one declaration with `--symbol NAME` or `--at PATH:LINE`, or select a scope with `--package`, `--module`, or `--root-path`. `callers` traverses incoming edges and `callees` outgoing edges; each displays source-written signatures by default. Repeatable language, confidence, and visibility filters apply before traversal. JSON retains declaration/call IDs, source facts, query roots, filters, and diagnostics. `--depth N` is bounded to 1–10, cycles are visited once, and ambiguous candidate targets remain explicit rather than guessed. Positional paths define the larger graph universe; remote queries accept `--repo OWNER/REPO[@REF] --server URL` for one exact indexed checkout.

## Directory architecture

`grepple architecture directory` provides bounded, source-linked directory orientation without inferring package or team intent. Use `--relations` for one compressed relation per directory pair, `--mermaid` to export a directory-level dependency diagram, and `--json` for the complete inventory and evidence. Scope it to the relevant directories; missing static edges do not prove that no runtime dependency exists.

```bash
grepple architecture directory --depth 2 .
grepple architecture directory --relations ./internal/cli
grepple architecture directory --mermaid --output architecture.mmd .
```

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

Matching is line-based for every readable non-NUL text file. Parser-backed features vary by language.

Run `grepple languages` for terminal feature and navigation-fact matrices or `grepple languages --json` for machine-readable capabilities. See the generated [language and feature support matrix](docs/file-type-support.md#current-support-matrix) for text grep, structural grep, outlines, navigation, focused structure/flow extraction, GritQL, and adapter-owned declarations, calls, imports, type references, fields, member access, and process entrypoints. Supported fact extraction is distinct from an individual fact remaining ambiguous or unresolved. Directory architecture applies to every language with parser navigation support.


## Agent workflow benchmarks

Run the fixed end-to-end discovery, navigation, impact, and edit-location benchmark with:

```bash
make agent-benchmark
```

It reports modeled retrieval calls, returned bytes, approximate tokens, elapsed time, and allocations while verifying answer fragments. See [Agent workflow benchmarks](docs/agent-workflow-benchmarks.md) for methodology and the current baseline. Run `make architecture-benchmark` for focused structure/flow metrics; reviewed thresholds are documented in [Architecture performance benchmarks](docs/architecture-performance-benchmarks.md).

## Repository configuration

Grepple discovers the nearest ancestor `grepple.json`. Repository-owned ignore paths apply consistently to recursive local search, graph queries, GritQL, and architecture discovery. Explicitly named files bypass ignores and emit a notice.

```json
{
  "server": "https://grepple.example.com",
  "ignore": {"paths": ["sandbox/**", "vendor/**"]},
  "output": {"spillThresholdBytes": 65536},
  "index": {
    "repositories": [
      {"repo": "sourcegraph/zoekt", "branches": ["main", "release/*"], "tags": ["v0.25.*"]}
    ]
  }
}
```

When this repository is indexed by the Grepple backend, its `index.repositories` declarations are trusted and reconciled automatically. A target's default branch is always indexed. Branch and tag patterns use path-style globs; matching tags are ordered by semantic version when possible and capped at the newest 20 per target. Use `grepple refs OWNER/REPO` to obtain an exact indexed selector for remote search, `get`, or `tree`. See [Versioned remote indexing](docs/versioned-indexing.md).

Inspect the effective source universe before drawing completeness conclusions:

```bash
grepple sources explain .
grepple sources explain --json --production-only .
```

`--no-config-ignore` keeps repository configuration but disables `ignore.paths`; `--no-repo-config` bypasses repository behavior without disabling user authentication. `--production-only` includes only files whose current `grepple.yaml` entry classifies them as `production`; files with missing, stale, absent, or invalid classification metadata are `unknown` and excluded. Explicit files still win and disclose the bypass. See [Repository source scope](docs/source-scope.md) for classification and omission semantics.

Complete output larger than the threshold is stored in the platform user cache (typically `~/.cache/grepple/output/`, with an automatic temporary-directory fallback) using a content-addressed filename; stdout receives a small `grepple-artifact-v1` descriptor. `GREPPLE_ARTIFACT_DIR` changes the default location. Use `--artifact-dir PATH` for one explicit directory, `--no-spill` when a script requires the original stream, or `--spill-threshold-bytes N` for one invocation. `grepple artifacts clean` removes the default retained artifacts; explicitly directed artifact directories remain caller-managed. Buffering and default spill storage stay outside the analyzed repository, allowing analysis of read-only checkouts. Repository configuration cannot contain or override authentication credentials stored in `~/.grepple/config.json`.

## Directory metadata

Use `grepple init` to create missing or refresh stale directory `grepple.yaml` files using the configured model. Local `grepple tree` shows current per-file `areas` tags and sorted unions of selected descendant tags on directories. Tree defaults to top-level files and directories; `--depth N` expands. Repeat `--area NAME` to keep files tagged with **any** requested area and their parent directories; `--kind` narrows those matches further. Shown files retain all their tags, including ones not used as filters. Stale or invalid tags never qualify a file; remote indexed trees cannot verify tags and reject `--area`. There is no top-level area or start command. `grepple verify --areas` reports stale or invalid memberships. See [Directory metadata](docs/directory-metadata.md).

```bash
grepple init --only-directory internal/parser
grepple verify
grepple tree                             # one level, with available areas
grepple tree --depth 2                   # expand one subtree
grepple tree --area navigation-resolution --area remote-search internal/cli
```

## Remote service

The distributed router, shards, repository synchronization, Zoekt integration, and deployment assets live in the private sibling `grepple-backend` repository. This public repository contains only the CLI and shared search contracts.

Search remains local-first. It contacts a remote service only when `--remote` (`-R`) or `--server` is supplied:

```bash
grepple "useEffect" src                         # local only
grepple --remote --repo owner/repo "useEffect"  # local plus configured service
grepple --server https://grepple.example.com --repo owner/repo "useEffect"
grepple repos                                    # indexed repository catalog
grepple get owner/repo path/to/file --lines 20:50
grepple tree internal/cli --depth 3              # local source tree
grepple tree --kind test --depth 2 internal/cli    # only checksum-validated test files and their parent directories
grepple tree --repo owner/repo src --depth 3     # indexed repository tree (no --kind)
```

Remote server URLs are resolved from `--server`, `GREPPLE_SERVER`, `./grepple.json`, `~/.grepple/config.json`, and finally `http://127.0.0.1:8787`.

### Login

A compatible service can advertise GitHub OAuth device-flow configuration. Authenticate and persist the resulting token with:

```bash
grepple login --url https://grepple.example.com
grepple logout
```

Remote search, `get`, `tree`, and rule commands send the stored bearer token. `GREPPLE_TOKEN` can provide an externally managed token instead.
