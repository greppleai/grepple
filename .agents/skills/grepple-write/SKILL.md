---
name: grepple-write
description: Use when editing, creating, or deleting local source and documentation files with Grepple. Covers fresh hashline anchors, inclusive replacements, literal multi-file transactions, dry runs, and safe retry after stale-anchor rejection. Use separate skills for discovery, change impact, and architectural decisions.
---

# Edit and write files with Grepple

Use `grepple write` for local repository changes. Retrieve source with Grepple first; do not rewrite an existing file just to avoid obtaining edit anchors. For an unfamiliar area, start with a bounded `grepple tree`, then focused search or `--outline`. Use `grepple-change-impact-analysis` before changing behavior, signatures, or ownership.

## 1. Retrieve fresh edit anchors

```sh
grepple --line-only -F 'targetName' path/to/package
grepple --line-only --at path/to/file.go:40-65
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
- After a write, formatter, generator, or any other mutation, retrieve fresh anchors before another edit. Do not reuse stale hashes, including hashes predicted by a dry run.
- If source is elided by session coverage, use `--repeat-source` on the focused `--at` read when you need the full replacement context. Truncated or spilled output is not a complete source read.

## 2. Write literal, quoted-heredoc transactions

```sh
grepple write --root . --dry-run <<'GREPPLE_WRITE_example'
::grepple file path/to/file.go
::grepple replace Ab3 Pq2
func newName() {
    newCall()
}
::grepple end
GREPPLE_WRITE_example
```

Inspect the dry-run diff, then apply the **same transaction** without `--dry-run` if its source is still current. Quoting the shell heredoc delimiter preserves quotes, backticks, and dollar signs literally. Replacement bodies contain source only, without anchor prefixes.

Multiple `replace` blocks can follow one `file` directive; add another `file` directive for another path. Keep related edits in one transaction so every operation is validated before any requested file changes.

### Insert or delete lines

There is no need to invent an insertion anchor: replace an existing anchored line with that original line plus the new lines before or after it.

```sh
grepple write --root . <<'GREPPLE_INSERT_example'
::grepple file path/to/file.go
::grepple replace K9x K9x
    oldCall()
    addedCall()
::grepple end
GREPPLE_INSERT_example
```

An empty replacement body deletes an inclusive range:

```sh
grepple write --root . <<'GREPPLE_DELETE_LINES_example'
::grepple file path/to/file.go
::grepple replace START END
::grepple end
GREPPLE_DELETE_LINES_example
```

Replace `START` and `END` with actual fresh hashes. A single anchor is allowed when both endpoints are the same line.

## 3. Create or delete whole files explicitly

Create only an absent path; missing parent directories are created automatically:

```sh
grepple write --root . <<'GREPPLE_CREATE_example'
::grepple file path/to/new.go
::grepple create
package sample

::grepple end
GREPPLE_CREATE_example
```

The blank body line before the terminator preserves the new file's final newline. Creation does not overwrite an existing file. Existing-file edits preserve its LF/CRLF style and final-newline behavior.

Whole-file deletion requires the current lowercase SHA-256 digest, **not a three-character line anchor**:

```sh
sha256sum path/to/obsolete.go
```

After inspecting the file and confirming deletion is intended, copy that exact digest into a transaction:

```sh
grepple write --root . <<'GREPPLE_DELETE_FILE_example'
::grepple file path/to/obsolete.go
::grepple delete CURRENT_64_CHARACTER_SHA256
GREPPLE_DELETE_FILE_example
```
Edits, creates, and digest-guarded deletes can share one transaction. A stale digest rejects the transaction.

### Escape a body terminator collision

If the content itself has an exact `::grepple end` line, choose a collision-free marker:

```sh
grepple write --root . <<'GREPPLE_CREATE_DOC_example'
::grepple file docs/example.md
::grepple create --end-marker DOC_BODY
Literal documentation containing this line:
::grepple end

::grepple end DOC_BODY
GREPPLE_CREATE_DOC_example
```
The marker option also works on `replace`. Only the selected exact terminator closes that body; other directive-looking lines inside it are literal content. Keep the shell heredoc delimiter distinct from the body marker and absent from the content.

## 4. Interpret receipts and retry safely

- Default success output is compact: `PATH:START-END|FIRST-LAST`, plus deletion/creation markers. It is not the complete resulting source. Do not reread merely to confirm text you supplied.
- Use `--return` when an immediate follow-up edit needs resulting `HASH│LINE│content` rows and nearby anchors. Otherwise retrieve only the changed range with `grepple --line-only --at PATH:START-END` when needed.
- On stale-anchor, overlap, path, or concurrent-mutation rejection, do not force the write or fall back to a blind overwrite. Read current source, reconsider the edit, and rebuild the transaction with fresh anchors. Error output may include fresh nearby anchors.
- Rejection changes no requested file. Installation uses staging and best-effort rollback; this is **not** a crash-proof filesystem journal. Do not claim power-loss atomicity.
- Run the applicable formatter, focused tests, and `git diff --check`. Refresh anchors after formatting. Update repository metadata/checksums and generated documentation when required. Do not commit or push without authorization.

## Safety and limits

The short `grepple write --help` synopsis may show only `edit`; the heredoc's `replace`, `create`, and digest-guarded `delete` directives are supported. Verify uncertain transactions with `--dry-run`, rather than assuming an operation is unavailable.

Writes are confined to `--root`; absolute paths, escapes, non-regular targets, and symlink escapes are refused. Requests are bounded to 16 MiB, source/result files to 16 MiB, and transactions to 128 files and 4,096 operations/changes. Existing mixed or bare-CR line endings and NUL bytes are rejected. Created files use mode `0644` and automatically created directories use mode `0755`, subject to umask and platform support.

Never use a remote search result or an unrelated anchor provider's hashes as local write authority. Use generic file-edit tools only if Grepple is unavailable or cannot represent the operation, then return to quoted-heredoc transactions.
