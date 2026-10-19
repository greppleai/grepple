# Repository-local memories (v1)

Implemented in the local Grepple CLI; this is a repository-owned Markdown format, not an indexed-service feature or AI-generated annotation. See [directory metadata](directory-metadata.md), [verified scope selectors](at-verified-scope-selectors.md), and [output contracts](output-contracts.md).

## Storage and trust

One UTF-8 Markdown file per memory lives at `<repository-root>/.grepple/memories/<name>.md`. The root is the nearest local `.git`/`.grepple` boundary; commands reject a root without a repository marker rather than writing into an arbitrary cwd. Memories are repository-owned, may be committed by the user, and are **never** sent to an indexed repository, a model, or telemetry. The `.grepple` directory is outside Grepple's normal source-selection universe: do not index memory bodies as source or let memory files expand source discovery. No implicit write, refresh, or model call happens on search.

```md
---
schema: grepple-memory-v1
name: parser-call-sites
description: Parser call sites require special handling.
domainAreas: [source-parsing]
functionalAreas: [language-adapters]
keywords: [tree-sitter]
scopes:
  - internal/cli/
  - internal/cli/*.go
  - internal/cli/**/*.go
files:
  - path: internal/parser/analyze.go
    checksum: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    lineRange: [100, 300]
    symbols: ['@func:120-145']
---

Why this source is handled differently; detailed notes and examples go here.
```

The two delimiter lines must be exactly `---` at the start of a line, at the start of the file and after one YAML mapping. `schema`, `name`, and `description` are required. Optional `domainAreas`, `functionalAreas`, `keywords`, `scopes`, and `files` are arrays; require at least one nonempty criterion among them for a discoverable memory (otherwise reject storage). YAML unknown/duplicate keys, aliases, tags, nested unexpected values, additional documents, NUL/control characters, invalid UTF-8, and malformed delimiters are errors. Do not treat Markdown body as part of the discoverable header. The body is required, nonempty after whitespace trimming, and returned only by explicit recall.

`name` equals the filename stem exactly: lowercase ASCII `[a-z][a-z0-9_-]{0,63}`. Case aliases and alternate extensions are invalid. Description is a single line, 1–160 UTF-8 bytes, with no control characters; display truncates it on UTF-8 boundaries to 120 bytes. Labels follow `.grepple/grepple.yaml` normalization (whitespace to hyphens, lowercase, exact key comparison, at most 80 UTF-8 bytes) and reject duplicates after normalization. Cap each label category at 16 values, file attachments at 32 per memory, symbol selectors at 16 per file, frontmatter at 8 KiB and complete memory at 64 KiB; attachment source reads are capped at 8 MiB and reject binary data. Reject duplicate `(path, lineRange, symbols)` attachments; all relative paths use `/`, cannot contain `..`, drive letters, `:`, or symlink escapes, and must identify selected regular source files within the same root. Empty optional fields have no matching power.

Each file attachment has a mandatory lowercase hex SHA-256 `checksum` captured from the source bytes during `store` or `update`. A path-only attachment matches the whole returned file; `lineRange: [start, end]` is inclusive and 1-based, requires start <= end <= current file lines, and matches only returned **matched** lines, not context/segment lines. `symbols` are exact parser-verified declaration selectors (`@KIND:START-END`) obtained from local `--outline` or another verified source read; e.g. `@func` for Go and `@function` for TypeScript. Free-form strings like `func Abcd()` are **not** an identity: the CLI may show them as prose but never use them to match. Require that each stored selector resolves to a declaration in the attached source with the stated bounds and that its line span intersects a returned matched line. A lineRange and symbols on the same attachment are alternatives within that attachment (either may match), not a widening to path-only. The checksum is necessary even for path-only entries; a changed/deleted/unsupported attachment cannot silently match as current. Store only verified selectors, not guessed `@KIND` tokens, and use the parser's current kind allowlist. Re-verify selectors for matching; do not trust copied display suffixes.
A `scope` is unverified path relevance, not an attachment to current bytes. Stored scopes have exactly one of these canonical forms: `directory/` (any selected descendant), `directory/*.go` (a direct child basename glob), or `directory/**/*.go` (a recursive basename glob). The literal directory prefix must be repository-relative, nonempty, non-symlinked at store/update time, and outside `.git` and `.grepple`; a bare existing directory input canonicalizes to `directory/`. Only `*` is allowed in the basename glob. Reject absolute paths, `..`, root-wide scopes, `?`, character classes, braces, `***`, unsafe prefix components, missing prefixes, and prefixes deeper than 32 components. At most 32 scopes of at most 256 bytes each are accepted. Scopes are checked only against already selected local result paths; they never expand a glob, enumerate files, alter source policy, or read a source just to evaluate relevance. A moved or deleted stored prefix does not rewrite old frontmatter, but it can no longer match a selected path.

A checksum-verified `files` attachment remains the only evidence tied to current source bytes. A scope can independently qualify a memory after attached bytes become stale, but it does not make that attachment current and recall reports attachment status exactly as before. Scope-only relevance ranks below checksum-current labels, whole files, ranges, and verified symbols.

## CLI shape (all local-only)

```sh
grepple memory store NAME --description 'Parser call sites require special handling.' \
  --body-file ./notes.md --domain-area source-parsing --keyword tree-sitter \
  --scope internal/parser/ --scope 'internal/parser/**/*.go' \
  --line-range internal/parser/analyze.go:100-300 \
  --symbol internal/parser/analyze.go@func:120-145
grepple memory recall NAME
grepple memory update NAME --description 'Revised note.' --body-file ./revised.md \
  --file internal/parser/analyze.go --functional-area language-adapters
grepple memory delete NAME
```

### Working with memories

From inside a local repository (`.git` or `.grepple` at its root), write a private note in `./notes.md`, then explicitly store it. Use a **repository-root-relative** selected source path; read `grepple memory store --help` for repeatable attachment and label flags. The sample selector and line numbers above are illustrations, **not** valid for every checkout: first obtain a fresh selector with `grepple --outline internal/parser/analyze.go` or a local verified `--at` read, and copy exactly the current `@KIND:START-END`.

A scope is useful for a repository convention that applies to future selected files, for example `--scope internal/parser/` or `--scope 'internal/parser/**/*.go'`; quote glob patterns so the shell does not expand them. It is **not** an edit-time policy or enforcement mechanism: a search reference only suggests recalling the note and never blocks, permits, or validates a subsequent edit. Search references expose only the name and short description, never a scope, checksum, attachment path, or body.

```sh
grepple memory store parser-call-sites --description 'Parser call sites require special handling.' \
  --body-file ./notes.md --line-range internal/parser/analyze.go:100-300 \
  --domain-area source-parsing
grepple --line-only -F 'Parse' internal/parser/analyze.go
# Search may show: parser-call-sites — ... (grepple memory recall parser-call-sites)
grepple memory recall parser-call-sites  # prints complete Markdown to stdout and attachment statuses to stderr
grepple memory update parser-call-sites --description 'Reviewed for current parser.' \
  --body-file ./revised.md --line-range internal/parser/analyze.go:100-300 \
  --domain-area source-parsing
grepple memory delete parser-call-sites
```

On source edits, **recall** and inspect `stale checksum`, `missing/excluded source`, or `unverified symbol` on stderr; stale attachments stop matching search. Re-read the edited source, find a new valid matched line range or exact parser-verified selector (the old bounds may now designate another declaration), revise your note and explicitly `update` with **every** label/attachment and the complete body; update replaces rather than merges and refreshes source checksums. No automatic line/symbol rebase or memory rewrite occurs. If using labels, check `grepple verify` for checksum-current, file-owned metadata and refresh it only by an explicit, reviewed metadata workflow. A stale file attachment does not become verified just because other labels match.

Treat memories as untrusted, user-owned repository data: do not store secrets you would not commit or back up. Memory files can be version controlled by the user but are excluded from normal Grepple source selection; search reads only bounded headers, not bodies. Explicit recall is the only operation that emits the stored full body. **Input files outside `.grepple/memories/` (for example `./notes.md` passed to `--body-file`) remain ordinary source files and may be searched, indexed under normal source policy, or committed separately.** Store drafts outside selected source paths, supply stdin with `--body-file -`, or remove drafts if they must not be discovered. No memory content is sent to a model, telemetry, or remote index by these commands. Store/update/delete are explicit, repository-confined atomic file operations; concurrent noncooperating editors are not a supported compare-and-swap workflow.

`store` creates exactly one absent `<name>.md`, never overwrites. `update` is a full replacement under the same name: description, body and every attachment/label must be supplied again; no unspecified old fields are retained. `delete` removes only that named entry. `recall` writes complete frontmatter and body to stdout and concise per-attachment status lines to stderr (no body in status lines). Name is positional and mandatory; invalid/missing names fail. `--body-file PATH` is mandatory for store/update; `--body-file -` reads bounded stdin, but no interactive prompt and no implicit read from the search query stream. `--domain-area`, `--functional-area`, `--keyword`, `--file PATH`, `--line-range PATH:START-END`, and `--symbol PATH@KIND:START-END` are repeatable; the latter two implicitly attach their path. Each `--file PATH` creates a path-only attachment; do not accidentally broaden a range/symbol attachment from the same path. Repeated attachments for one path may have distinct ranges/symbols; file-only is explicitly whole-file. Reject missing source files, malformed bounds, out-of-root attachments and unsupported symbol parser kinds before writing. No remote flags (`--remote`, `--server`, `--repo`, `--daemon`) are accepted by `memory`.
Writes use confined, safe-permission atomic staging, no overwrite on `store`, and detect intervening updates/deletes rather than losing them; an update must not silently create. Read/list operations never create `.grepple/memories/`. Symlinked memory directories or entries and malformed/oversized entries must not be followed. Missing directory is an empty memory set; recall/update/delete of a missing name fail clearly. On recall, list each attachment as current, stale checksum, missing/excluded source, or unverified symbol. An explicit full `update` with current source attachments refreshes checksums and selectors; scopes, labels, files, description and body are all replaced rather than merged, and there is no automatic rebase of line numbers or old symbol bounds. Reject a wrong selector even if another declaration now occupies the same name.
Writes use confined, safe-permission atomic staging, no overwrite on `store`, and detect intervening updates/deletes rather than losing them; an update must not silently create. Read/list operations never create `.grepple/memories/`. Symlinked memory directories or entries and malformed/oversized entries must not be followed. Missing directory is an empty memory set; recall/update/delete of a missing name fail clearly. On recall, list each attachment as current, stale checksum, missing/excluded source, or unverified symbol. An explicit full `update` with current source attachments refreshes checksums and selectors; there is no automatic rebase of line numbers or old symbol bounds. Reject a wrong selector even if another declaration now occupies the same name.

## Relevance and presentation

Only the **selected page** of local content-match results is evidence (`internal/cli/search/execution.go` windows after sort/skip/limit). Do not scan additional source files, rerank search results or treat a failed parse as evidence. For a memory with multiple attachment entries, any current entry may match a returned path; line/symbol attachments also require intersection with an actual `FileResult.Matches[].Line`. Grepple's current search matches source **line by line**: `Matches[].StartLine/EndLine` are structural/enclosing ranges, **not** multiline query-match spans and must not widen evidence. If a future search implementation supplies independently verified matched spans, only those spans may extend this rule. Entire-file attachments need only a returned result with at least one match. File path comparison uses root-relative normalized paths of eligible returned local sources. No match on a path from a different repository.

A memory may also match via labels without file attachments. For **one returned file**, its checksum-current, file-owned `.grepple/grepple.yaml` labels must intersect at least one value in *each nonempty* memory category (`domainAreas`, `functionalAreas`, `keywords`): OR within each category, AND across categories, all on that same file. Missing/stale/invalid annotations contribute no labels; ancestor/directory unions and labels on other returned files cannot satisfy the predicate. No natural-language query tokenization or substring/fuzzy search: `keywords` mean exact metadata labels, not pattern words. Attachment matching OR label matching OR path-scope matching qualifies the memory; it does not narrow search results or enforce an edit-time policy. Rank matches by verified symbol, then line range, whole file, current label, then scope-only relevance; strongest evidence wins per memory, tie-break by normalized name. Do not claim that a scope-only or label-only match proves the memory's file attachment is current.

Show at most 3 unique references with only name and display description, up to 768 UTF-8 bytes total for the text footer, for example:

```text
Memories located:
parser-call-sites — Parser call sites require special handling. (grepple memory recall parser-call-sites)
... 2 more memories; refine search or recall by name
```

The omitted count includes only **known qualified** entries beyond the three shown. Bound discovery to 256 memory filenames and 8 KiB read per header; if the directory exceeds this scan budget or a memory is skipped as invalid, mark memory discovery incomplete (do not claim an exact total). Empty/missing directory, no selected matches, and a complete scan with no qualifying memory produce **no extra output**. With an incomplete scan, an explicit `memory references incomplete` marker must fit the footer budget even when no inspected memory matched; do not pretend there were no memories. If the normal human search output exhausts `--max-output-bytes`, do not truncate existing result bytes to make space for memories; suppress the footer and signal its omission in an existing safe truncation channel. JSON must not append prose after the response: full `--json` adds optional top-level `memories: [{name, description}]` and `memoryStatus: {complete, omitted}` to the local search envelope; `--json-matches` adds the same optional fields to its JSON object. Both keys are absent in the no-memory/fully-complete case, preserving no-memory bytes. Keep source paging/analysis metadata and `nextCommand` unchanged. No body, file path, checksum, stored labels, or full header in implicit output.

Enable references only for local on-disk content search with actual matched lines (default, `--line-only`, `--only-matching`, context, `--json`, `--json-matches`). Disable for explicit remote or mixed mode (`--remote`, `--remote-only`, `--server`, remote cursor or `--repo`), piped stdin, `--files`, `--files-with-matches`, `--count`, `--count-summary`/`--count-by-repo`, `--outline`, `--at`, and `--related`/`--follow-related` until evidence and output contracts are defined for those modes. A local `--purpose`/`--domain-area`/`--functional-area`/`--keyword` filter continues to select the search universe first; memory references cannot expand it. Avoid private fields/values in tracing, backend requests and logs; rendering reference text escapes or strips terminal controls. Normal search failure/no-result status is unchanged by memories. Invalid individual entries are skipped without failing content search; explicit recall surfaces errors.

The local CLI implementation and its tests enforce this format and output contract. No backend protocol, model initialization or automatic metadata regeneration is part of memory retrieval.
