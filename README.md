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
`same-file-struct-methods` analyzer uses tree-sitter Go syntax trees and requires every method of a struct to live in the file that declares the type.
The PreToolUse grep guard is also implemented in the standalone module and uses the tree-sitter Bash grammar to inspect actual command invocations.

Mermaid extraction and validation live in the main `extract` package and are exposed through focused `grepple extract structure|flow` commands; the hooks module imports that package for automatic Stop validation rather than maintaining a second analyzer. Language-neutral repository orientation is generated dynamically through `grepple architecture directory|resolve|why|responsibilities`, while `architecture compare` diagnoses drift between complete reports, so no package/workspace artifacts need to be committed under `.grepple/`. Graph, architecture, boundary, and responsibility analysis can target one exact indexed checkout through `--server` and `--repo`. `make schema-generate` and `make schema-check` validate parser-generated language metadata. The Stop hook validates `*.class.mmd`, `*.structure.mmd`, and `*.flow.mmd` schemas.

Public parser consumers should treat `Document` as the owning parse boundary, `Node` as a document-tied handle, `DocumentView`/`ViewNode` as callback-scoped lock-free traversal, and `SyntaxNode` as the persistent immutable snapshot. See [Parser syntax lifecycle](docs/parser-syntax-lifecycle.md) for ownership, locking, invalidation, and snapshot guidance.

The `grepple` application binary is built into `bin/grepple`.

The default Linux build links the CGO tree-sitter runtime and all grammars into self-contained static binaries. Command-line parsing uses [`go-arg`](https://github.com/alexflint/go-arg). Delegated research uses [`charm.land/fantasy`](https://github.com/charmbracelet/fantasy), currently pinned to the newest release compatible with Go 1.25.

## Delegated research

Authenticate a capable, usually cheaper retrieval model once, then delegate broad source exploration or batch reads without consuming repeated turns or filling the calling agent's context:

```bash
grepple ai-provider login codex
grepple ask Which package owns navigation resolution and what calls it?
```

The internal agent retrieves and sifts multi-file evidence; it is not a code-review or approval agent. Its `read_file` tool can batch up to eight known ranges, and local source rows retain `HASH│LINE│content` when user anchor settings are enabled so the main coding agent can edit without another read. Focused typed tools also cover text search, exact navigation, structural search, graph queries, architecture, source scope, and indexed refs and trees. They call Grepple internals directly: no generic argv tool, command parser, executable subprocess, or shell is exposed to the model.

Successful identical typed calls within one ask reuse byte-identical evidence and concurrent duplicates share one execution; cache metadata and JSONL events make this observable. Logs also include response-free per-tool timing records with full typed inputs and a session performance summary separating merged tool wall time from estimated LLM-facing stream time. Local navigation, graph, and architecture tools additionally share one lazily parsed source universe whenever their effective scope agrees. Ask logs are permission-restricted, enabled by default, and retain managed logs for seven days unless `ask.logs` changes that policy. The provider registry supports Codex and GitHub Copilot device login, Anthropic API and subscription tokens, OpenAI API keys, and the AWS Bedrock default credential chain. Set the non-secret user default as `ask.model: "<provider>/<model>"` in `~/.grepple/grepple.json`; the same prefix form works with `--model`. See [Delegated research](docs/ask.md) for provider, model, logging, timeout, credential, timing, and tool-safety details.

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
- `--follow-related N` (expand up to two unique callers and callees per level, depth 1-3; structural search defaults to depth 1)
- `--skip N`, `--limit N` (page through results in deterministic order: skip the first `N` files / return at most `N`; **`--limit` defaults to `20`**, use `--limit 0` for all). A server never returns more than **100 files per page** — `--limit 0` or a larger value gets the maximum page, and you page further with `--skip N`; local-only searches stay uncapped
- `--sort path|matches` keeps repository/path order by default or opts into matching-line count descending with repository/path tie-breakers. Match-count sorting scans the full selected candidate universe before paging.

### Grep migration and editing workflow

Grepple is not a complete GNU/BSD grep flag emulation layer. Patterns use JavaScript regular-expression syntax by default; `-E` is accepted as an explicit alias for that default, while `-r` is accepted because filesystem searches are already recursive. Use `-F` when the pattern must be literal. Other unsupported grep flags should be translated to Grepple's output model rather than copied mechanically.

For agent edits, Grepple can either locate lines for a normal Read step or emit harness-compatible edit anchors through a user-configured provider:

```text
grepple --line-only or structural search → Read exact PATH:LINE or PATH:START-END → Edit with hash anchors
grepple search (with anchors enabled in settings) → Edit directly with emitted HASH│LINE│content rows
```

`HASH` is the edit key and `LINE` is the 1-indexed orientation/fallback location used by Read and `--at`. Eligible local structural, contextual, and line-only searches always use anchors, including bounded `-C`/`-A`/`-B` context and exact `--at PATH:START-END --line-only` retrieval. Grepple's built-in `hashline-v1` implementation is the default. `grepple write` auto-detects either one strict `grepple-write-v1` JSON request or a literal `::grepple file` heredoc transaction on stdin, and prevalidates anchored edits plus explicit transactional creates and digest-guarded deletes across confined paths. Heredoc transactions provide the same multi-file features without JSON or shell escaping and support caller-selected `--end-marker` terminators for directive-looking source. For one edit, `grepple write edit --path PATH --start HASH [--end HASH]` reads literal replacement text from stdin or `--content-file`. Write responses are edit-ready `HASH│LINE│content` with freshly recomputed anchors; use `--json` for structured automation, and `--dry-run` for deterministic unified diffs plus predicted anchors. Named provider commands remain available for editor compatibility, execute without a shell, and use the versioned batch protocol. `grepple anchors setup` and `anchors doctor` manage those optional providers. See [Transactional hashline writes](docs/write.md) and [Edit-anchor providers](docs/anchor-providers.md).

Default structural output shows complete enclosing functions and methods, retaining only required parent wrappers and excluding proximity-selected imports, neighboring declarations, and nonmatching siblings. Plain `--line-only` output remains one row per match, but parser-backed source lines that begin a multi-line construct use `PATH:START-END:text` (for example a function or `if` statement) so the next Read can retrieve its exact extent; ordinary and unparsed lines retain `PATH:LINE:text`. Add `--enclosing` to annotate a body match with its nearest multi-line syntax scope as `PATH:MATCH@START-END:text`, such as `worker.go:122@121-123: save()`. Anchor rows preserve the exact `HASH│LINE│content` editing contract, so `--enclosing` automatically avoids default anchors and rejects explicitly requested anchors. Broad human-readable output is bounded before common tool-result limits and ends with narrowing guidance; tighten the path/glob or use `--limit`, `--line-only`, bounded context, `-l`, or `--count`.

### Source call navigation

Structural search automatically adds bounded navigation hints with one level of caller/callee locations for local or remotely indexed repositories; use `--no-related` to suppress navigation or `--follow-related N` to select depth 1-3. Human output keeps callers and callees as a compact call tree with exact `PATH:LINE` selectors instead of rendering their source bodies. It supports Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, C, C++, Rust, and Shell:

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

## Normalized navigation graph

`grepple graph --json [PATH...]` emits a deterministic `grepple-navigation-graph-v7` document from the same parser-owned facts used by `--at`, `--related`, and directory architecture. Callable nodes include stable IDs, language, repository-relative paths, declaration line ranges, package/module/container context, callable kinds, adapter-evidenced entrypoint roles, and language-specific visibility details where required. Source-declared type facts include normalized names, kinds, complete line ranges, and package/module identity; callable type usages retain parameter, receiver, local, and result roles. Calls include caller IDs, resolved target IDs, deterministic candidate target IDs, receiver/import context, line locations, and resolution confidence. Typed field/property, module re-export, and scoped Rust module facts support cross-file member chains, embedded/promoted Go methods, TypeScript/TSX inheritance, named/default aliases, barrel re-exports, nearest-`tsconfig.json` `baseUrl`/`paths` aliases, Rust inline or direct normal/raw-string `#[path]` ownership, and exact source-relative quoted C/C++ includes. Rust import and call resolution enforces syntax-evidenced private, crate, parent, self, and ancestor-restricted visibility while leaving external crates and conditional ownership unresolved. C/C++ angle-bracket includes, macro-computed paths, and compiler/build include directories remain unresolved. Resolution remains syntax-based and preserves ambiguity rather than claiming compiler dispatch. Every graph, focused query, diff side, and boundary report includes discovered, selected, parsed, skipped, failed, and recovery-parse source counts. JSON is never byte-truncated; `--max-files N` applies deterministic source-file truncation and records it separately.

Per-file parser facts are cached as deterministic path-neutral packed-protobuf entries under `.grepple/cache/navigation/` by default. The v26 codec serializes the native graph directly through a shared string table and packed columns, validates a payload checksum, bounds entry and column sizes, and atomically writes immutable content-addressed `.pb` files. Cache failures remain non-authoritative, and cold/warm graph output is identical.

```bash
grepple graph --json .
grepple graph --json ./internal/cli --max-files 200
grepple graph --compact ./internal/cli
grepple graph callers --at internal/cli/extract.go:32 --depth 2 --language go --compact .
grepple graph resolve --symbol runExtract --compact ./internal/cli
grepple graph callees --symbol runExtract --depth 2 --json ./internal/cli
grepple graph dependencies --root-path internal/cli --depth 1 --compact .
grepple graph impact --symbol runExtract --depth 2 --compact ./internal/cli
grepple graph diff --before ./old-tree --after ./new-tree --compact
grepple graph impact --server http://127.0.0.1:8080 --repo OWNER/REPO@tag~v1.2.3 --symbol Run --compact
```

`--compact` emits a bounded agent-facing declaration/edge list with 16-character stable ID prefixes, source locations, arrow direction, confidence, aggregate `resolved-local`, `ambiguous-local`, `unresolved-local`, and `expected-external` outcome counts/rates, source-completeness totals, and truncation warnings; `--max-output-bytes` defaults to 16384 and never applies to JSON. Full JSON reports the same authoritative outcomes separately from deterministic confidence-label counts, overall and by language. Legacy resolved/ambiguous/unresolved/candidate fields remain additive compatibility projections and do not redefine `candidate` confidence as an outcome. Unsupported file types are counted as skipped before the file limit. Files containing NUL are skipped, hard read/UTF-8/parse failures are counted as failed, and recovery parses are counted separately while retaining their conservative graph facts. Complete JSON retains unresolved/external calls with `candidate` confidence and ordered candidate IDs; compact output omits calls with no repository-local target and retains ambiguous calls without guessing one target. Exactly one of `--json` or `--compact` is required.

CLI graph, focused graph query, `--related`, boundary, and extraction workflows share parser-owned per-file navigation facts under `.grepple/cache/navigation/`. Entries are addressed by source content, language, grammar ABI/fingerprint, and a cache schema; path-neutral facts are instantiated with each command's requested path so stable IDs and output remain unchanged. Corrupt or unwritable entries are ignored, writes are atomic, and cold and warm reports are byte-identical. Set `GREPPLE_NAVIGATION_CACHE_DIR` explicitly to relocate the cache or to an empty value to disable it.

`graph resolve --symbol NAME` performs no traversal. It previews all exact-name matches, or terminal-name matches when there is no exact match, with full stable declaration IDs, exact `PATH:LINE` selectors, and copyable callers/callees/impact commands. Repeatable language and visibility filters narrow alternatives before selection. Complete `grepple-navigation-resolve-v1` JSON or bounded compact output is required, making overloaded or cross-container names cheap to disambiguate before a graph query.
`graph callers`, `callees`, `dependencies`, `dependents`, and `impact` return deterministic subgraphs over repository-local navigation edges. Select one exact declaration with `--symbol NAME` or `--at PATH:LINE`, or select a scope with `--package`, `--module`, or `--root-path`; scope selectors may produce multiple roots. `callers` and `dependents` traverse incoming call/navigation edges, while `callees` and `dependencies` traverse outgoing call/navigation edges; `dependencies` and `dependents` are navigation terminology and do not describe package-manager, module, or build-system dependency graphs. `impact` traverses both directions. Repeatable `--language ID`, `--confidence LEVEL`, and `--visibility LEVEL` filters apply before root selection and traversal; confidence accepts `exact`, `import-resolved`, `context-resolved`, `unique-terminal`, and `candidate`, while visibility accepts `public`, `non-public`, and `unknown`. JSON and compact declarations expose visibility and entrypoint facts. `unknown` visibility is retained where a language lacks reliable public/export semantics. `--depth N` is bounded to 1–10, cycles are visited once, and ambiguous candidate targets remain explicit rather than being guessed. JSON retains the `grepple-navigation-graph-v7` facts and adds normalized query direction, depth, root IDs, and filters; compact mode adds concise query metadata to its header. Positional paths define the larger graph universe, while the root selector chooses where traversal starts. `--repo OWNER/REPO[@REF] --server URL` runs the same complete-source graph and query over one exact indexed checkout; remote JSON wraps the versioned result with repository, commit, completeness, notices, and shard errors.

`graph diff` compares two source trees using `grepple-navigation-diff-v5`. It classifies added, removed, moved, and semantically changed declarations plus added, removed, and changed calls. Position-only line shifts are ignored, and call owners are compared through semantic declaration identities rather than unstable source IDs. Exactly one of complete `--json` or bounded `--compact` is required.

## Boundary analysis

`grepple boundaries [PATH...]` reports repeated owner-file workflows, concrete type spread, owned field/property surface, and policy-backed facade bypasses. Spread is classified as `package-internal`, `cross-package`, `cross-layer`, or `public-api`; containment is `approved`, `escaped`, or `unknown`. Type usage distinguishes public API, private signatures, field representation, and body-local use. Repository policy can identify layers, containment rules, facade/implementation path sets, and intentional utility, test-framework, declarative-configuration, lifecycle-cleanup, or adapter-protocol paths. Without policy, same-directory ownership facts can establish approved package-internal use, but cross-boundary intent remains unknown. Signals are conservative review leads, never policy-independent violations.

```bash
grepple boundaries parser
grepple boundaries ./src ./lib --min-occurrences 3
grepple boundaries --json ./internal
grepple boundaries --policy .grepple/boundary-policy.json --json .
grepple boundaries --server http://127.0.0.1:8080 --repo OWNER/REPO@branch~main --json
```

Human output is bounded to 16,384 bytes and shows at most 20 ranked candidates per workflow/type/facade section by default; evidence and omissions remain explicit. `--limit 0` shows all candidates, while `grepple-boundaries-v3` JSON is complete. Optional parser facts use `GREPPLE_NAVIGATION_CACHE_DIR`; resolved boundary graph inputs default to the platform user cache under a repository-keyed `grepple/cache` directory and can be relocated with `GREPPLE_CACHE_DIR`. Read-only analysis does not create `.grepple` beneath the target repository. Use `--no-cache` to bypass boundary and parser cache layers. `--max-files` remains explicit when source discovery is incomplete. Remote boundaries load `.grepple/boundary-policy.json` from the selected checkout by default; `--policy` remains repository-relative and malformed policy fails explicitly.

## Directory architecture and focused diagrams
`grepple architecture` provides deterministic, language-neutral directory orientation over every Tree-sitter-backed language. `directory` summarizes physical ownership and separately labeled call/import/type relations, `resolve` locates types and callables with exact ranges, `why` returns source-linked evidence for one directory relation, `responsibilities` summarizes each directory's ownership plus incoming/outgoing relation participation, and `compare` diagnoses semantic or byte-level drift between complete directory JSON reports. Relation coverage preserves unresolved and adapter-unsupported semantics. Directory ownership is intentionally not presented as package, module, or layer intent. See [Directory architecture](docs/directory-architecture.md) for schemas and evidence limits.

`grepple extract` creates deterministic, self-validated focused Mermaid navigation maps from Tree-sitter source analysis. Focused structure and flow extraction support Go, JavaScript/JSX, TypeScript/TSX, Python, Java, Kotlin, C#, Rust, C, and C++. Python covers classes, inheritance, annotated and unannotated attributes, decorators, and `.pyi` stubs. Java, Kotlin, and C# cover their class/interface models, inheritance, fields/properties, methods, records/data classes, enums, objects where applicable, and conservative language-defined process-entrypoint signatures. Rust covers structs, tuple structs, enums, traits, same-file and visibility-checked same-crate cross-file `impl` blocks, fields, variants, associated types, methods, visibility, trait implementations, graph-backed calls, and top-level `main` process entrypoints in selected binary crate roots without interpreting framework APIs. C and C++ conservatively cover named aggregates, fields, enum values, direct base classes, functions, methods, visibility, static members, graph-backed calls, global `main` process entrypoints, and direct include evidence while leaving macro-computed and system include resolution, templates, overload selection, linkage, and build-system ownership uninterpreted. Generic callable declarations and calls use the normalized `parser.NavigationGraph` shared with `--at` and `--related`.

```bash
grepple architecture directory --depth 2 --compact .
grepple architecture resolve --symbol Document --compact .
grepple architecture why rulespec search --compact rulespec search
grepple architecture responsibilities --compact .
grepple architecture directory --server http://127.0.0.1:8080 --repo OWNER/REPO --compact
grepple extract structure internal/cli --entry cliOptions --source .
grepple extract flow internal/cli --entry runSearch --depth 2
grepple extract flow --at internal/cli/local.go:13 --source .
grepple extract structure --at web/store.ts:8 --source web
grepple extract flow --at services/worker.py:20 --source services
grepple extract structure --at src/main/java/acme/Service.java:12 --source .
grepple extract flow --at src/main/kotlin/acme/Worker.kt:30 --source .
```

`architecture directory` uses explicit depth, node, file, and output bounds in compact mode; complete JSON may spill to a disclosed artifact according to repository output policy. `architecture resolve` indexes declaration outlines rather than only callable graph nodes. `architecture why` currently reports only `exact`, `import-resolved`, and `context-resolved` static calls, so absence does not prove that no build-system, reflective, registration, or runtime dependency exists. `architecture responsibilities` is a physical ownership summary, not an inferred team or business-domain assignment. Directory, resolve, why, and responsibilities accept `--server URL --repo OWNER/REPO[@REF]`; exact checkout identity and commit are retained in remote JSON. `architecture compare` first normalizes paths and collection order, reports the first source-linked semantic difference, and only then diagnoses raw-byte drift.

Focused extraction requires `--entry` or `--at PATH:LINE`. `flow` follows statically resolved outgoing calls. `--source` can be repeated to define analysis roots; `--depth`, `--max-nodes`, and `--output` control projection and writing. A flow that reaches `--max-nodes` remains valid and deterministic, and includes `%% grepple:truncated max-nodes N` instead of failing after useful nodes have already been selected.

Every generated type/function node includes its exact `PATH:START-END` definition range; type notes also list exact member ranges. Flow nodes display their definition range directly. Generated diagrams are checked against the same source analysis before being returned.

Validation is available through `grepple extract check structure|flow`. `extract.SupportedLanguages` and `extract.LanguageForPath` expose focused adapter metadata. To add a focused language, implement one adapter with its Tree-sitter analyzer and structure/flow projections, then add it to `registeredLanguages`; directory architecture uses parser capabilities directly.

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

Grepple discovers the nearest ancestor `grepple.json`. Repository-owned ignore paths apply consistently to recursive local search, graph, boundary, GritQL, focused extraction, and architecture discovery. Explicitly named files bypass ignores and emit a notice.

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
grepple sources explain --compact .
grepple sources explain --json --production-only .
```

`--no-config-ignore` keeps repository configuration but disables `ignore.paths`; `--no-repo-config` bypasses repository behavior without disabling user authentication. `--production-only` excludes conventionally classified tests, fixtures, generated sources, and vendored sources during recursive discovery. Explicit files still win and disclose the bypass. See [Repository source scope](docs/source-scope.md) for classification and omission semantics.

Complete output larger than the threshold is stored in the platform user cache (typically `~/.cache/grepple/output/`, with an automatic temporary-directory fallback) using a content-addressed filename; stdout receives a small `grepple-artifact-v1` descriptor. `GREPPLE_ARTIFACT_DIR` changes the default location. Use `--artifact-dir PATH` for one explicit directory, `--no-spill` when a script requires the original stream, or `--spill-threshold-bytes N` for one invocation. `grepple artifacts clean` removes the default retained artifacts; explicitly directed artifact directories remain caller-managed. Buffering and default spill storage stay outside the analyzed repository, allowing analysis of read-only checkouts. Repository configuration cannot contain or override authentication credentials stored in `~/.grepple/config.json`.

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
grepple tree --repo owner/repo src --depth 3     # indexed repository tree
```

Remote server URLs are resolved from `--server`, `GREPPLE_SERVER`, `./grepple.json`, `~/.grepple/config.json`, and finally `http://127.0.0.1:8787`.

### Login

A compatible service can advertise GitHub OAuth device-flow configuration. Authenticate and persist the resulting token with:

```bash
grepple login --url https://grepple.example.com
grepple logout
```

Remote search, `get`, `tree`, and rule commands send the stored bearer token. `GREPPLE_TOKEN` can provide an externally managed token instead.
