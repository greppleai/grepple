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

## Architecture artifacts

Canonical generated architecture documentation is under `.grepple/`:

- `.grepple/project.workspace/overview.mmd`: project overview
- `.grepple/<package>.package/overview.mmd`: compact package view
- `.grepple/<package>.package/structure.mmd`: exhaustive package structure
- `.grepple/<package>.package/manifest.json`: machine-readable model

Do not edit generated artifacts manually. Run `make schema-generate` after structural changes and `make schema-check` to verify them.

## Validation

Format Go code with `gofmt` and run `make test` before finishing.
