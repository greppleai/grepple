# Repository file annotations

Grepple stores one repository-owned `.grepple/grepple.yaml`. Its strict format is `schema: v1` with a singular `directory` map keyed by repository-relative slash paths (`.` is the root). Each entry has `files` with directory-relative **basenames**. No per-directory YAML or old `version`/`directories` format is read. Unknown YAML keys, duplicate names/keys, path escapes and malformed checksums are errors.

```yaml
schema: v1
directory:
  internal/parser:
    files:
      - name: file.go
        checksum: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
        description: Dispatches source parsing.
        purpose: implementation
        domainAreas: [source-parsing]
        functionalAreas: [cli-navigation]
        keywords: [c++]
```

`name`, SHA-256 `checksum`, short nonempty `description` (up to 240 bytes) and `purpose` are required for annotated files. Suggested purposes are **documentation**, **implementation**, **testing**, **configuration**, **ci-cd**, **agent-skills**, **examples** and **fixture**. They describe a file's primary role: use `ci-cd` for pipeline/workflow definitions and `agent-skills` for agent skill/instruction files, not for every implementation file about those features. Put the subject in `domainAreas` instead. This is not an enum; other source-grounded purposes (for example `vendor`) are allowed. `domainAreas`, `functionalAreas` and `keywords` are optional and may be omitted or empty. Labels are stored as lowercase kebab-case: whitespace runs become `-`, while punctuation such as `c++` is retained. Input labels must be nonempty UTF-8 of at most 80 bytes without control characters. Selector values accept either `source parsing` or `source-parsing` and match exactly after the same normalization (no stemming or substring matching). Output order is deterministic; descriptions and file names retain their original spelling. No directory description, responsibility, kind, aggregate category or area proposal is stored: directory labels are the union of selected, checksum-current file labels in that directory **and its descendants**. Unselected files, missing metadata and stale checksums never contribute trusted labels.

```sh
grepple init --only-directory internal/parser  # opt-in bounded model generation; replace path
grepple verify --json                        # review current/stale/missing coverage
grepple overview                             # inspect actual labels and file counts; no model call
grepple tree --purpose Implementation --depth 2 internal/parser
grepple --purpose Implementation --line-only -F 'Parse' internal/parser
# If a file changed, opt into refreshing only that area, then verify again:
grepple init --only-directory internal/parser
grepple init --force --only-directory internal/parser  # deliberately regenerate unchanged annotations too
```

`init` runs the configured model only when explicitly invoked and a selected directory needs generation (or `--force` is supplied). It supplies the selected source manifest, preserves Grepple-computed file checksums, and atomically updates the shared YAML without losing sibling directory entries. `--concurrency N` bounds simultaneous jobs; output remains ordered. A refresh replaces selected files' descriptions and purposes, retains previously reviewed labels for selected files whose checksums are unchanged, and preserves checksum-current preexisting annotations for files excluded by the active source scope. Removed or stale excluded annotations are pruned; excluded files cannot expand generation's selected manifest or influence filtered search. `verify` reports missing, stale and invalid selected directories with per-directory reasons and fails when coverage is incomplete.

Source selection, `.gitignore`, repository ignores and `--production-only` remain authoritative. Unknown classification can exclude files when `--production-only` is active: inspect the source scope and choose a bounded `init --only-directory PATH` without that restriction before opting into generation. Unfiltered `grepple init` can call the configured model across the entire checkout; run it only with explicit approval. `overview` uses the same selected-file inventory as local tree and verify; it counts only checksum-current file annotations when displaying purpose, domain, functional and keyword vocabulary. Each category shows at most eight labels (sorted by descending file count and then normalized label) and two example paths per label. Text and JSON both report complete distinct counts and explicit omitted counts, with selected/current/stale/missing/invalid file totals. Missing or invalid YAML yields honest coverage and `grepple verify`/bounded `init` guidance, never fabricated labels. Overview is local-only and does not contact a model. Its selector examples use `--purpose`, `--domain-area`, and `--keyword` with tree or local search.

Local tree accepts repeatable `--purpose`, `--domain-area`, `--functional-area` and `--keyword`. Values within a category OR; requested categories AND on one selected, checksum-current file. Matching uses normalized exact labels (for example `c++`), not substrings. Directory entries require matching descendants, even if `--depth 1` hides those files; their labels are unions of only matching descendants. Text and JSON include at most two sorted examples per category alongside the complete totals (text shows `+N`). A filtered no-match tree returns empty entries and exits 1 in both modes, printing `grepple overview`/`grepple verify` guidance to stderr. Missing, invalid, stale and ignored files cannot match. Remote indexed trees reject all four selectors; the obsolete `--area` and `--kind` tree flags are no longer accepted.

Local `grepple search` (also the default command) accepts the same four repeatable exact metadata selectors. They narrow source-policy-selected candidate files before content scans, structural rendering, sorting, `--skip`/`--limit`, and complete count summaries. Supported outputs include default and line-only search, `--files`, `--files-with-matches`, `--count`, `--count-summary`, JSON/JSON matches, `--outline`, and explicit `--at`; nonmatching `--at` fails before reading its target. For example: `grepple --purpose Implementation --keyword 'c++' --line-only pattern internal` or `grepple --domain-area 'Source parsing' --files internal`. If no current annotations match, results remain empty and stderr suggests `grepple overview` or `grepple verify`; no unfiltered fallback occurs. Path/glob selection and existing source policy constrain candidates first; directly named files retain the source policy's explicit ignore/production bypass behavior, but must still have matching current metadata. Explicit `--related`/`--follow-related` are rejected with metadata filters until navigation can use the selected-file universe; automatic related navigation is disabled. Remote/mixed `--remote`, `--remote-only`, `--server`, `--repo`, cursor paging and piped stdin are not supported with metadata selectors. Other entrypoints (`graph`, `grit`, `architecture`, `ask`, `get`, `refs`, `rules`) have separate command parsers and do **not** apply search metadata selectors; use exact local search/outline/at or unfiltered structural/graph commands instead. Overview and search never call a model.