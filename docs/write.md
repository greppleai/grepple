# Transactional hashline writes

`grepple write` applies one prevalidated transaction across edits, creates, and deletions. It uses Grepple's native `hashline-v1` implementation, so anchors from local source retrieval and optional `write --return` rows can be reused without an editor-specific adapter.

## Anchored edit workflow

Retrieve an exact range or bounded context:

```text
grepple --line-only --at path/to/file.go:40-65
grepple --line-only -C 2 -F 'old value' path/to/file.go
grepple --line-only --at path/to/file.go @func:40-65 # Verify current kind/bounds before rereading clean source
```

An annotated `--line-only` **search** may append display-only `@KIND:START-END` suffixes after literal source. Those decorated rows are not edit-ready even if they show hashes. For a parser-verified local scope, `grepple --line-only --at PATH @KIND:START-END` rechecks the current file and returns clean source; missing, stale, ambiguous or uncertain scopes fail. For literal bounds without scope validation, use `grepple --line-only --at PATH:START-END` instead. Only write from fresh, clean, complete local source rows; a truncated/spilled response is not an inline source read. Anchor rows are exactly:

```text
HASH│LINE│content
```

Submit a strict JSON request on stdin:

```sh
grepple write --root . <<'JSON'
{
  "schema": "grepple-write-v1",
  "files": [{
    "path": "path/to/file.go",
    "changes": [{
      "hash_range_inclusive": ["abc", "def"],
      "content_lines": ["replacement", "lines"]
    }]
  }]
}
JSON
```

Ranges are inclusive and always refer to the original file snapshot. `content_lines: []` deletes a range. To insert around an existing line, replace that line with the original line plus the inserted lines. Multiple changes in one file may be supplied in any order but cannot overlap.

## Literal multi-file transactions

For multi-edit and multi-file work, `grepple write` accepts a line-oriented literal envelope. Each operation is a single directive; its body is literal text and is not JSON-escaped or interpreted by the shell:

```sh
grepple write --root . <<'GREPPLE_WRITE_ab12'
::grepple replace internal/foo.go a83 def
func foo() {
    return "anything ' ` $ here"
}
::grepple replace internal/foo.go 81b
::grepple create internal/new.go
package internal

::grepple delete internal/old.go 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
GREPPLE_WRITE_ab12
```

The supported operation directives are `::grepple replace PATH START [END]`, `::grepple create PATH`, and `::grepple delete PATH SHA256`. `START` and optional `END` are fresh case-sensitive line hashes; the range is inclusive and omitting `END` means a one-line replacement. Delete uses the exact current lowercase 64-character SHA-256 digest. Replace and create consume literal body lines until the next line beginning with `::grepple` or end-of-input (EOF). There is no `::grepple end` terminator, `--end-marker` option, or inferred-end warning.

Paths with spaces are supported, including create paths. Replace uses the last three-character hash as an anchor and treats a preceding three-character token as an optional end anchor; delete uses the final digest. A replace path ending in a space-separated three-character hash-shaped component is ambiguous: use strict JSON for that path. Because any `::grepple` directive line ends the current body and starts directive parsing, the envelope cannot represent body content that looks like a directive. Use strict JSON for such content, or use `grepple write edit` for a single edit.

Each replacement in a file refers to that file's original snapshot; multiple replacements must not overlap and must not adjust later anchors for prior changes. Multiple operations can appear in any order and are prevalidated together. A blank line immediately after a body directive is body content, not envelope whitespace. Include a final blank body line for create when the new file should end in a newline; omitting it creates a file without a final newline. An empty replacement body deletes its anchored range.

The envelope and strict JSON compile to the same in-memory `grepple-write-v1` request. Validation, confinement, stale checks, overlap rejection, mixed edit/create/delete prevalidation and installation, output modes, staging, concurrent-mutation checks, and rollback are identical.

Fresh hashes are recomputed against the complete resulting file. Compact receipts use `PATH:START-END|FIRST-LAST` for replacements and creates, `PATH:deleted at LINE (near LINE|HASH)` for range deletions with surviving context (or just the insertion point when the file becomes empty), and `PATH:deleted`, `PATH:created empty`, or `PATH:unchanged` for whole-file operations and no-op edits. Each change gets its own line. Use `grepple --line-only --at PATH:START-END` to retrieve content when needed. With `--return`, replacement rows include neighboring anchors for orientation, and deletions return surviving neighbors. The complete structured `--json` response still includes content-bearing anchors and `after_sha256`.

Each source retrieval independently renders its selected complete anchored rows; earlier searches and `write --return` responses do not suppress later source. Explicit output-size, spill, and paging limits still apply. Compact write receipts intentionally contain no source; use `--return`, a focused `grepple --line-only --at PATH:START-END` read, or structured `--json` when source is needed.

## Structured output

Use `--json` when a script needs the complete `grepple-write-v1` response:

```sh
grepple write --root . --json < request.json
```

JSON includes operation, before/after SHA-256 digests, request-ordered change details, resulting line ranges, deletion markers, and actual post-write anchors. Automation needing this response must pass `--json` explicitly; ordinary writes print compact human receipts, with edit-ready rows opt-in via `--return`. Large complete responses use the normal content-addressed spill mechanism unless `--no-spill` is requested. Writes have no preview mode: inspect anchors and content first, then submit one prevalidated transaction. A rejected request changes no requested files; installation failures receive best-effort rollback.

## Creating files

Creation is explicit and only targets an absent repository-relative path beneath the confined root. Missing parent directories are created automatically:

```json
{
  "path": "path/to/new.go",
  "operation": "create",
  "content_lines": ["package sample", ""]
}
```

The equivalent literal operation is `::grepple create path/to/new.go`, followed by its body. A blank body line is significant: include one as the final body line to preserve a final newline in the created file; without it, the file has no final newline.

Each array entry is one logical line. A trailing empty entry preserves a final newline. Created files use mode `0644` and automatically created directories use mode `0755`, both subject to the process umask and platform support. New directories participate in transaction rollback: a later installation failure removes files and directories created by the transaction in reverse order. Existing targets, symlink escapes, parent paths that are files, embedded newlines, NUL bytes, and oversized results are rejected.

## Deleting files

Deletion requires the exact digest of the current file and does not accept changes or file content:

```json
{
  "path": "path/to/old.go",
  "operation": "delete",
  "before_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

The equivalent literal operation is `::grepple delete path/to/old.go SHA256`.

A stale digest rejects the entire transaction. Deletions are installed through the same backup sequence as replacements and are restored if a later installation fails.

## Transaction contract

Before mutation, Grepple validates every operation, path, filesystem identity, file type, digest, anchor, range, replacement line, overlap, newline style, and size limit. Duplicate lexical paths and hardlink identities are rejected. Changed content is staged on the destination filesystem before missing create directories are made, existing bytes are rechecked for concurrent changes, creates are installed without replacing a target that appeared concurrently, and existing files are backed up before installation. A failed installation removes files and directories created by the transaction and restores backups.

A rejection changes no requested file and exits with status 1. Human errors include concise path/change context and fresh nearby anchor rows when available. `--json` returns a structured error with the same evidence. Rollback failures remain visible in the error rather than being discarded.

As with any multi-file operation on ordinary filesystems, an operating-system crash or power loss during the final installation sequence is not a general filesystem transaction. Grepple provides complete prevalidation and best-effort rollback, not a cross-filesystem journal.

## Safety and limits

- `--root` defaults to the working directory.
- Request paths must be relative and resolve inside the root.
- Existing file and directory symlink escapes, directories, non-regular files, and missing edit/delete targets are refused.
- Create requires an absent target; missing parent directories beneath the confined root are created automatically.
- Delete requires a lowercase 64-character SHA-256 digest.
- Existing LF or CRLF style and final-newline behavior are preserved by edits.
- Mixed or bare-CR source files and NUL-containing existing files are refused.
- Content entries cannot contain embedded newline characters.
- Request size is capped at 16 MiB; each source or result file is capped at 16 MiB.
- A transaction supports at most 128 files and 4,096 operations/changes.
- No shell, configured anchor provider, or model is executed by `grepple write`.
