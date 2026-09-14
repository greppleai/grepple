# AGENTS.md

## Project

Grepple is the public Go code-search engine and CLI. Public packages are importable by the private sibling backend:

- `api`: dependency-free HTTP DTOs
- `parser`: language detection, tree-sitter parsing, syntax documents, segments, and outlines
- `search`: discovery, matching, filtering, paging, and result construction
- `gritql`: native, bounded structural query compilation and evaluation
- `gritqlapi`: conversion from structural results to API DTOs
- `rulespec`: shared text and structural saved-rule validation
- `internal/cli`: repository-local CLI workflows and output rendering

Keep dependencies directed toward `api` and `parser`. Distributed router, shard, and repository-management code lives in `../grepple-backend`; this public module must not depend on it.

Language-specific parser and navigation syntax belongs behind the owning `languageAdapter`, with parsing entered through `languageAdapter.Parse()` and navigation policy exposed through `languageAdapter.Navigation()`. `syntaxTree` and `syntaxNode` are the private structural boundary; production parser files outside `tree_sitter.go` must not import or name go-tree-sitter types. Shared parser engines must not contain canonical language IDs or grammar node-kind policy. Architecture tests enforce these boundaries.

## Architecture discovery

Inspect source scope before architecture when exclusions or production/test composition can matter:

- `grepple sources explain --compact [PATH]`: config digest, classifications, exclusions, and explicit bypasses

Architecture orientation is generated dynamically across supported languages:

- `grepple architecture directory --compact [PATH]`: bounded directory ownership and relation map
- `grepple architecture resolve --symbol NAME --compact [PATH]`: exact source-linked declaration lookup
- `grepple architecture why FROM TO --compact [PATH]`: resolved cross-directory call evidence

Do not infer package semantics from directory ownership. Use `--production-only` only for explicitly production-scoped questions; retain full-universe evidence otherwise. `make schema-generate` and `make schema-check` validate generated parser metadata; architecture views do not require committed package/workspace bundles.

## Validation

Format Go code with `gofmt` and run `make test` before finishing.
