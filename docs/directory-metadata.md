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

Generate or refresh metadata with the model configured for `grepple ask`:

```bash
grepple init                           # create missing and refresh stale/invalid metadata
grepple init --concurrency 4           # process up to four directories in parallel
grepple init --only-directory internal/parser
grepple init --force --only-directory internal/parser  # regenerate even if current
grepple verify
grepple verify --json
```

`init` runs a read-only agent for each missing, stale, or invalid directory; current metadata is skipped, and a no-op refresh needs no model credentials. The agent can list the Grepple-selected directory tree, search it with Grepple, and read bounded source ranges with line numbers. It assigns every file one evidence-backed kind from `production`, `test`, `fixture`, `generated`, `vendor`, or `unknown`. Existing `grepple.yaml` is supplied as prior context for a refresh. Grepple preserves deterministic file paths and SHA-256 checksums instead of trusting model output for those fields. `--force` regenerates all selected directories, even current ones. `--concurrency N` (default `1`; `N` must be positive) bounds simultaneous directory agents; output follows directory order and failures are reported without discarding successful updates to other directories. Source selection follows `.gitignore`, repository ignores, and `--production-only`. Use `--only-directory PATH` to process exactly one directory without generating metadata for its ancestors. Run an unfiltered `init` when refreshing stale classifications: `--production-only` excludes unknown files.

`verify` checks every selected source directory, including the repository root and intermediate directories. It reports missing, invalid, and stale metadata and exits with status 1 when verification fails. Metadata is stale when selected files are absent from `grepple.yaml`, recorded files no longer exist, or a recorded SHA-256 checksum no longer matches current content. Missing kinds remain backward-compatible and normalize to `unknown`; non-empty values outside the supported enum make metadata invalid.

Local `grepple tree` appends available directory and file descriptions. Use `grepple tree --kind test --depth 2 PATH` to see only test-classified files and their ancestor directories; the same local-only flag accepts `production`, `fixture`, `generated`, `vendor`, and `unknown`. Kinds are trusted only when their recorded checksum matches current content. `unknown` includes unclassified, stale, and missing metadata entries; `--kind` cannot be combined with `--production-only` for non-production kinds, and is unavailable for indexed remote trees, which lack file-kind metadata. Text and JSON output contain only matching file entries and their parent directories; a tree with no matches has an empty entry list. Metadata freshness still reflects the unfiltered selected directory. The unfiltered local tree marks affected entries with `[metadata: missing]`, `[metadata: stale]`, or `[metadata: invalid]`; current metadata is intentionally unmarked. JSON tree responses expose `metadataStatus` and `metadataIssues` on the root and entries so agents can avoid treating stale descriptions as authoritative. `grepple ask` receives a depth-two local directory summary in its system prompt before invoking tools.