# grepple

Structure-aware grep for agents, implemented in Go. Search works across text files, while supported programming-language files receive compact tree-sitter structural context. Results are returned in deterministic path order (there is no relevance ranking) so you refine by narrowing with filters rather than trusting a score.

## Project layout

```text
.grepple/          Canonical machine-generated architecture artifacts
api/               Dependency-free HTTP request and response contracts
cmd/grepple/       Minimal executable entry point
docs/              User and file-type documentation
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

Docker-capable hosts can run the opt-in native-only image gate with `make docker-smoke`. It builds and exercises the final image, runs a structural query, verifies that Node/npm/npx, Rust/Cargo, and upstream `grit`/`gritql` executables are absent, and removes the temporary image. This check is intentionally separate from `make test`.

Linting uses [revive](https://github.com/mgechev/revive) with the pinned rule set in `revive.toml` (the documented default rules, made explicit, plus `cognitive-complexity` capped at 15):
`make lint` runs revive plus the standalone pi hook module's lint and tests. Pi executes a cached `hooks/bin/pi-hook` binary; the lightweight Make targets in `.pi/settings.json` rebuild it only when its Go sources or module files change. The Stop hook runs
on every agent Stop: it auto-fixes formatting with `gofmt -w`, then feeds one revive rule-group (worst file first, one
rule at a time) back to the agent with a remediation guide from `hooks/guides/<rule>.md` until the tree is clean.
Project analyzers in `hooks/internal/pihooks/` add repo-specific checks that revive cannot express. The current
`same-file-struct-methods` analyzer uses tree-sitter Go syntax trees and requires every method of a struct to live in the file that declares the type.
The PreToolUse grep guard is also implemented in the standalone module and uses the tree-sitter Bash grammar to inspect actual command invocations.

The hooks module validates and generates Mermaid class/structure and call-flow schemas from Go and TypeScript/TSX. This repository exports every generated architecture artifact to the root [`.grepple/`](.grepple/) directory; `make schema-generate` rewrites them there and `make schema-check` reads and verifies them there. Go package generation supports both the legacy single diagram and canonical three-file bundles: `mermaid-code generate package <source-directory> --format bundle --output <bundle-directory>` and `mermaid-code check package <bundle-directory> [source-directory]`. It also generates canonical two-file project inventories with `mermaid-code generate workspace <root> --output <bundle-directory>` and validates them with `mermaid-code check workspace <bundle-directory> [root]`. Bundle manifests carry normalized project-relative source metadata, so checks can self-locate their source. The Stop hook automatically validates legacy `*.class.mmd`, `*.structure.mmd`, and `*.flow.mmd` schemas plus canonical `*.package/manifest.json` and `*.workspace/manifest.json` bundles. Generated output is excluded from source analysis, while Stop discovery still enters `.grepple/` to validate it. Generation and checking use `hooks/bin/mermaid-code`. See [`hooks/README.md`](hooks/README.md) for commands, bundle artifacts, metadata, and discovery rules.

The `grepple` application binary is built into `bin/grepple`.

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
