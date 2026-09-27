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
    areas: [syntax-parsing] # optional, repository-owned area membership
```

Generate or refresh metadata with the model configured in user settings:

```bash
grepple init                           # create missing and refresh stale/invalid metadata
grepple init --concurrency 4           # process up to four directories in parallel
grepple init --only-directory internal/parser
grepple init --force --only-directory internal/parser  # regenerate even if current
grepple verify
grepple verify --areas                 # flag stale/invalid annotated file memberships
grepple verify --json
```

`init` runs a read-only agent for each missing, stale, or invalid directory; current metadata is skipped, and a no-op refresh needs no model credentials. The agent can list the Grepple-selected directory tree, search it with Grepple, and read bounded source ranges with line numbers. It assigns every file one evidence-backed kind from `production`, `test`, `fixture`, `generated`, `vendor`, or `unknown`. On a fresh repository with no existing tags, it is asked to independently propose reusable feature areas from inspected source and tests; no area names are built into the implementation, and files without meaningful feature evidence may remain untagged. Existing `grepple.yaml` is supplied as prior context for a refresh. Even for `--only-directory`, the agent receives a repository-wide, source-scope-aware inventory of existing area tags as hints, including stale statuses; this inventory is not proof of semantic ownership. Default sequential init refreshes that inventory between directories, allowing later directories to reuse newly written areas when source evidence supports them. Parallel init uses one initial snapshot to avoid timing-dependent prompts. New model-suggested areas require a matching `area_proposals` entry containing a selected file path, area, `action: add`, and concrete `evidence` (source file and line with reason). Valid hand-authored tags are retained even if omitted by the model; a proposed removal (`action: remove`) is printed for review but never automatically removes a tag. Proposals are not stored in `grepple.yaml`. Grepple preserves deterministic file paths and SHA-256 checksums instead of trusting model output for those fields. `--force` regenerates all selected directories, even current ones. `--concurrency N` (default `1`; `N` must be positive) bounds simultaneous directory agents; output follows directory order and failures are reported without discarding successful updates to other directories. Source selection follows `.gitignore`, repository ignores, and `--production-only`. Use `--only-directory PATH` to process exactly one directory without generating metadata for its ancestors. Run an unfiltered `init` when refreshing stale classifications: `--production-only` excludes unknown files.

`verify` checks every selected source directory, including the repository root and intermediate directories. It reports missing, invalid, and stale metadata and exits with status 1 when verification fails. Metadata is stale when selected files are absent from `grepple.yaml`, recorded files no longer exist, or a recorded SHA-256 checksum no longer matches current content. Missing kinds remain backward-compatible and normalize to `unknown`; non-empty values outside the supported enum make metadata invalid. `areas` is optional on file entries; each tag must start with a lowercase letter and contain only lowercase letters, digits and single interior hyphens. Invalid or repeated tags invalidate the entry. `verify --areas` also emits file-specific tagged membership leads whose checksums are stale or whose tags/kind/description are invalid, in human or JSON output. It never guesses missing or incorrect semantic ownership from absent call edges.

Area tags are optional repository-owned metadata. Local `grepple tree` displays current tags on files and the sorted union of selected descendant tags on each directory, including the root. `grepple tree --area NAME PATH` filters files to one area; repeating `--area` matches **any** requested tag. Directories appear only when they contain matching files (possibly below the requested depth). Combine `--kind` to require both the kind and an area match. The displayed tags include all current tags on matched files, not only the requested ones. No area is inferred for an untagged, stale, or invalid file; indexed remote trees reject the filter because their metadata freshness cannot be verified. The default depth is one level; `--depth N` expands subtrees.
Local `grepple tree` appends available directory and file descriptions. Use `grepple tree --kind test --depth 2 PATH` to see only test-classified files and their ancestor directories; the same local-only flag accepts `production`, `fixture`, `generated`, `vendor`, and `unknown`. Kinds are trusted only when their recorded checksum matches current content. `unknown` includes unclassified, stale, and missing metadata entries; `--kind` cannot be combined with `--production-only` for non-production kinds, and is unavailable for indexed remote trees, which lack file-kind metadata. Text and JSON output contain only matching file entries and their parent directories; a tree with no matches has an empty entry list. Metadata freshness still reflects the unfiltered selected directory. The unfiltered local tree marks affected entries with `[metadata: missing]`, `[metadata: stale]`, or `[metadata: invalid]`; current metadata is intentionally unmarked. JSON tree responses expose `metadataStatus` and `metadataIssues` on the root and entries so agents can avoid treating stale descriptions as authoritative.