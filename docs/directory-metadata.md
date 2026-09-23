# Directory metadata

Grepple uses a repository-owned `grepple.yaml` in each selected source directory to provide fast architectural orientation.

```yaml
description: Handles source parsing and syntax normalization.
responsibilities:
  - Parse supported source languages.
  - Normalize syntax trees.
  - Own Tree-sitter integration.
files:
  - path: file.go
    description: Handles parser dispatch.
    kind: production
    checksum: 0123456789abcdef
```

Generate missing metadata one directory at a time with the model configured for `grepple ask`:

```bash
grepple init
grepple init --only-directory internal/parser
grepple init --force --only-directory internal/parser
grepple verify
grepple verify --json
```

`init` runs a read-only agent for each directory. The agent can list the Grepple-selected directory tree, search it with Grepple, and read bounded source ranges with line numbers. It assigns every file one evidence-backed kind from `production`, `test`, `fixture`, `generated`, `vendor`, or `unknown`. When `grepple.yaml` already exists, `--force` supplies it as prior context so the agent can improve it. Grepple preserves deterministic file paths and SHA-256 checksums instead of trusting model output for those fields. Existing files are skipped unless `--force` is supplied. Source selection follows `.gitignore`, repository ignores, and `--production-only`. Use `--only-directory PATH` to process exactly one directory without generating metadata for its ancestors.

`verify` checks every selected source directory, including the repository root and intermediate directories. It reports missing, invalid, and stale metadata and exits with status 1 when verification fails. Metadata is stale when selected files are absent from `grepple.yaml`, recorded files no longer exist, or a recorded SHA-256 checksum no longer matches current content. Missing kinds remain backward-compatible and normalize to `unknown`; non-empty values outside the supported enum make metadata invalid.

Local `grepple tree` appends available directory and file descriptions. It marks affected entries with `[metadata: missing]`, `[metadata: stale]`, or `[metadata: invalid]`; current metadata is intentionally unmarked. JSON tree responses expose `metadataStatus` and `metadataIssues` on the root and entries so agents can avoid treating stale descriptions as authoritative. `grepple ask` receives a depth-two local directory summary in its system prompt before invoking tools.