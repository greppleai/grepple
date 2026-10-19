# AGENTS.md

## Project

Grepple is the public Go code-search engine and CLI. Only `api` is a supported library import for the private sibling backend; all implementation and shared wire packages live under `internal/`:

- `api`: outward-facing HTTP contract aliases and backend service facade
- `internal/wire`: shared transport DTOs used by internal CLI and HTTP adapters
- `internal/parser`: tree-sitter adapters, syntax facts, segments, and outlines
- `internal/navigation`: source-backed relationship graphs and dependency resolution
- `internal/search`: discovery, matching, filtering, paging, and result construction
- `internal/gritql`: native, bounded structural query compilation and evaluation
- `internal/gritqlapi`: conversion from structural results to internal wire DTOs
- `internal/rulespec`: shared text and structural saved-rule validation
- `internal/apiclient`: typed remote API boundary, authentication, HTTP execution, and response decoding
- `internal/cli`: repository-local CLI workflows and output rendering

Keep dependencies directed from `api` toward `internal/wire` and implementation owners. Internal packages must never import `api`; transport adapters use `internal/wire` and core engines keep transport out where possible. Distributed router, shard, and repository-management code lives in `../grepple-backend`; this public module must not depend on it.

Language-specific parser and navigation syntax belongs behind the owning `languageAdapter`, with parsing entered through `languageAdapter.Parse()` and navigation policy exposed through `languageAdapter.Navigation()`. `syntaxTree` and `syntaxNode` are the private structural boundary; production parser files outside `tree_sitter.go` must not import or name go-tree-sitter types. Shared parser engines must not contain canonical language IDs or grammar node-kind policy. Architecture tests enforce these boundaries.

## Architecture discovery

Inspect source scope before architecture when exclusions or production/test composition can matter:

- `grepple sources explain [PATH]`: config digest, classifications, exclusions, and explicit bypasses

For a new local session, run model-free `grepple overview` **first** to inspect checksum-current label vocabulary and coverage. Then run `grepple tree --depth 1` and expand a promising subtree. When a real label is known and current, use repeatable `grepple tree --purpose implementation --domain-area LABEL PATH` or local `grepple --purpose implementation --domain-area LABEL --files PATH`; OR within a category and AND categories on the same selected file. Missing/stale metadata never widens a filtered query: use unfiltered tree/search and `grepple verify` to inspect gaps. The former tree `--area` and file-classification `--kind` flags are gone (search `--outline --kind functions` is unrelated). Graph, architecture and grit are separate entrypoints, not scoped by these selectors. Follow tree leads with exact `--outline`/`--at` and focused graph queries rather than expanding the whole repository tree; do not run model-backed `grepple init` merely to orient.

Architecture orientation is generated dynamically across supported languages:

- `grepple architecture directory [PATH]`: bounded physical directory ownership and relation map
- `grepple --outline PATH`: declaration discovery for an exact file
- `grepple graph resolve --symbol NAME [PATH]`: disambiguate callable declarations
- `grepple graph callers|callees --at PATH:LINE [PATH]`: focused static call relations

Do not infer package semantics from directory ownership. Use `--production-only` only for explicitly production-scoped questions; retain full-universe evidence otherwise. `make schema-generate` and `make schema-check` validate generated parser metadata; architecture views do not require committed package/workspace bundles.

## Validation

Format Go code with `gofmt` and run `make test` before finishing.

## Commits

When asked to commit or propose a commit message, load the repository-local [semantic-commits skill](.pi/skills/semantic-commits/SKILL.md) and use Conventional Commit subjects. This is an agent-only project skill: keep it under `.pi/skills`, not the distributable Grepple CLI or `.agents/skills` plugin bundle. Do not commit or push without explicit authorization.
