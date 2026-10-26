---
name: grepple-write
description: Use when editing, creating, or deleting local source and documentation files with Grepple. Covers fresh hashline anchors, inclusive replacements, literal multi-file transactions, prevalidation, and safe retry after stale-anchor rejection. Use separate skills for discovery, change impact, and architectural decisions.
---

# Edit and write files with Grepple

Use `grepple write` for local repository changes. Retrieve source with Grepple first; do not rewrite an existing file just to avoid obtaining edit anchors. For an unfamiliar area, start with a bounded `grepple tree`; if local metadata is current, use an observed domain-area or functional-area label to narrow that tree and any broad discovery search before `--outline`. When the target file is known, prefer its emitted `@KIND:START-END` token and a verified local `--at` for declaration anchors; use a literal `--at PATH:START-END` when no verified token applies or only a small portion is needed. Adding a metadata label to an exact known-file read is redundant. Use `grepple-change-impact-analysis` before changing behavior, signatures, or ownership.

## 1. Retrieve fresh edit anchors

```sh
grepple --outline path/to/file.go # copy the current @func:START-END token for a Go function
grepple --line-only --at path/to/file.go @func:40-65 # only when the outline emitted these exact bounds
grepple --line-only --at path/to/file.go:40-65 # literal fallback: no parser verification
```

Eligible local output contains native `hashline-v1` rows:

```text
Ab3│40│func oldName() {
K9x│41│    oldCall()
Pq2│42│}
```

- The three-character **HASH** is the anchor. Copy `Ab3` and `Pq2`, not line numbers, `HASH│LINE│` prefixes, or the entire row. Hashes are case-sensitive; never invent them. The hashes above are illustrative: use those from your actual read.
- Line numbers locate source; they are not write anchors. Paths in a transaction must be repository-relative beneath `--root`.
- Replacement ranges are **inclusive**. Replacing this whole function requires both its opening line and closing brace; replacing only its body uses the body's anchors.
- All changes to one file in a transaction refer to the **same original snapshot**. Ranges must not overlap. Do not adjust later ranges for earlier replacements in that transaction.
- After a write, formatter, generator, or any other mutation, retrieve fresh anchors before another edit; do not reuse stale hashes.
- Annotated `--line-only` **search** rows may append display-only `@KIND:START-END` after source. They are not edit-ready even when they display hashes: literal source can also end with that text. Copy the **native token** from `--outline` (Go `@func`, TypeScript `@function`/`@method`) or an annotated hit, then pass it separately to `grepple --line-only --at PATH @KIND:START-END` to verify current syntax and obtain clean source. Do not guess a token from graph or remote ranges. If a verified selector is missing, stale, ambiguous, or inappropriate for the small region needed, use `--at PATH:START-END` for a literal read and assess its bounds yourself. Typed JSON separates `text` from `nearest` and `scope`.
- Focused `--at` and eligible local source reads return the complete selected source on every call, subject to explicit output, spill, and paging limits. A truncated or spilled output is not a complete inline source read.

## 2. Write literal, quoted-heredoc transactions

```sh
grepple write --root . <<'GREPPLE_WRITE_example'
::grepple replace path/to/file.go Ab3 Pq2
func newName() {
    newCall()
}
::grepple create path/to/new.go
package sample

::grepple delete path/to/obsolete.go CURRENT_64_CHARACTER_SHA256
GREPPLE_WRITE_example
```

Each operation is one directive: `::grepple replace PATH START [END]`, `::grepple create PATH`, or `::grepple delete PATH SHA256`. Replace consumes literal source lines through the next line beginning `::grepple` or EOF; create does likewise with complete new-file content. Delete has no body. There is no `::grepple end` terminator, `--end-marker`, or inferred-end warning. The quoted shell heredoc delimiter preserves quotes, backticks, and dollar signs literally. Review edit-ready source and requests before submitting; all requested changes are prevalidated before mutation.

Paths with spaces are supported, including create paths. The final hash/digest fields are parsed as anchors; for replace, a preceding three-character hash-shaped token is interpreted as `START` when two anchors are present. A replace path ending in a space-separated three-character hash-shaped component is ambiguous: use strict JSON for it. Use fresh, case-sensitive hashes; replacement ranges are inclusive. Multiple replacements in one file all refer to the same original snapshot and must not overlap. Do not shift later anchors to account for earlier replacements. Keep related operations in one transaction so every operation is validated together.

Any line beginning with `::grepple` ends the active body and is parsed as an operation directive, so directive-looking body lines cannot be represented in this envelope. Use strict JSON for such content, or use `grepple write edit` for a single edit. Blank lines are body content, including a blank line immediately following a directive; they are not ignored envelope whitespace. Include a final blank body line in a create operation to preserve the new file's final newline. An empty replace body deletes the anchored range.

### Insert or delete lines

There is no need to invent an insertion anchor: replace an existing anchored line with that original line plus the new lines before or after it.

```sh
grepple write --root . <<'GREPPLE_INSERT_example'
::grepple replace path/to/file.go K9x K9x
    oldCall()
    addedCall()
GREPPLE_INSERT_example
```

An empty replacement body deletes an inclusive range:

```sh
grepple write --root . <<'GREPPLE_DELETE_LINES_example'
::grepple replace path/to/file.go START END
GREPPLE_DELETE_LINES_example
```

For an empty body, EOF closes it; the next operation directive can alternatively close it. Replace `START` and `END` with actual fresh hashes. A single anchor is allowed when both endpoints are the same line.

## 3. Create or delete whole files explicitly

Create only an absent path; missing parent directories are created automatically:

```sh
grepple write --root . <<'GREPPLE_CREATE_example'
::grepple create path/to/new.go
package sample

GREPPLE_CREATE_example
```

The blank body line after `package sample` preserves the new file's final newline. Creation does not overwrite an existing file. Existing-file edits preserve their LF/CRLF style and final-newline behavior.

Whole-file deletion requires the current lowercase SHA-256 digest, **not a three-character line anchor**:

```sh
sha256sum path/to/obsolete.go
```

After inspecting the file and confirming deletion is intended, copy that exact digest into a transaction:

```sh
grepple write --root . <<'GREPPLE_DELETE_FILE_example'
::grepple delete path/to/obsolete.go CURRENT_64_CHARACTER_SHA256
GREPPLE_DELETE_FILE_example
```

Edits, creates, and digest-guarded deletes can share one transaction. A stale digest rejects the transaction.

## 4. Interpret receipts and retry safely

- Default success output is compact: `PATH:START-END|FIRST-LAST`, plus deletion/creation markers. It is not the complete resulting source. Do not reread merely to confirm text you supplied.
- Use `--return` when an immediate follow-up edit needs resulting `HASH│LINE│content` rows and nearby anchors. Otherwise retrieve only the changed range with `grepple --line-only --at PATH:START-END` when needed.
- On stale-anchor, overlap, path, or concurrent-mutation rejection, do not force the write or fall back to a blind overwrite. Read current source, reconsider the edit, and rebuild the transaction with fresh anchors. Error output may include fresh nearby anchors.
- Rejection changes no requested file. Installation uses staging and best-effort rollback; this is **not** a crash-proof filesystem journal. Do not claim power-loss atomicity.
- Run the applicable formatter, focused tests, and `git diff --check`. Refresh anchors after formatting. Update repository metadata/checksums and generated documentation when required. Do not commit or push without authorization.

## Safety and limits

The short `grepple write --help` synopsis may show only `edit`; the heredoc's `replace`, `create`, and digest-guarded `delete` directives are supported. Read the transaction examples and validate anchors and request shape before submitting: writes have no preview mode.

Writes are confined to `--root`; absolute paths, escapes, non-regular targets, and symlink escapes are refused. Requests are bounded to 16 MiB, source/result files to 16 MiB, and transactions to 128 files and 4,096 operations/changes. Existing mixed or bare-CR line endings and NUL bytes are rejected. Created files use mode `0644` and automatically created directories use mode `0755`, subject to umask and platform support.

Never use a remote search result or an unrelated anchor provider's hashes as local write authority. Use generic file-edit tools only if Grepple is unavailable or cannot represent the operation, then return to quoted-heredoc transactions.
