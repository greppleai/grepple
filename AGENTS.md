# AGENTS.md

## Project

Grepple is a Go code-search system. The main packages are under `internal/`:

- `api`: dependency-free HTTP DTOs
- `parser`: language detection, tree-sitter parsing, segments, and outlines
- `search`: discovery, matching, filtering, paging, and result construction
- `grepplecli`: CLI workflows and output rendering
- `router`, `shard`, `repository`: distributed search and repository management

Keep dependencies directed toward `api` and `parser`; do not make server packages depend on `grepplecli`.

## Architecture artifacts

Canonical generated architecture documentation is under `.grepple/`:

- `.grepple/project.workspace/overview.mmd`: project overview
- `.grepple/<package>.package/overview.mmd`: compact package view
- `.grepple/<package>.package/structure.mmd`: exhaustive package structure
- `.grepple/<package>.package/manifest.json`: machine-readable model
- `.grepple/shard-lifecycle.flow.mmd`: shard lifecycle flow

Do not edit generated artifacts manually. Run `make schema-generate` after structural changes and `make schema-check` to verify them.

## Validation

Format Go code with `gofmt` and run `make test` before finishing.
