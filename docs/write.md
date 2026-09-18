# Transactional hashline writes

`grepple write` applies one prevalidated transaction across edits, creates, and deletions. It uses Grepple's native `hashline-v1` implementation, so rows emitted by anchored search and successful writes can be reused without an editor-specific adapter.

## Anchored edit workflow

Retrieve an exact range or bounded context:

```text
grepple --line-only --at path/to/file.go:40-65
grepple --line-only -C 2 -F 'old value' path/to/file.go
```

Anchor rows are always exactly:

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

For multi-edit and multi-file work, `grepple write` also auto-detects a line-oriented `::grepple` envelope. Literal bodies are not JSON-escaped or interpreted by the shell:

```sh
grepple write --root . <<'GREPPLE_WRITE_ab12'
::grepple file internal/foo.go
::grepple replace a83 def
func foo() {
    return "anything ' ` $ here"
}
::grepple end

::grepple replace 81b
::grepple end

::grepple file internal/new.go
::grepple create
package internal

::grepple end

::grepple file internal/old.go
::grepple delete 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
GREPPLE_WRITE_ab12
```

`::grepple file PATH` starts a file block; the complete remainder of that directive is the path, so spaces do not require JSON quoting. `replace START [END]` starts a literal replacement and defaults `END` to `START`. Multiple replacements may follow one file directive. An empty body deletes the anchored range. `create` supplies complete literal content for an absent file. `delete SHA256` supplies the same required digest as JSON and has no body. A following file directive implicitly closes the previous file; a standalone `::grepple end` may explicitly close a bodyless delete/file block.

The default body terminator is an exact `::grepple end` line. When source itself contains that line, select a collision-free terminator with `--end-marker TOKEN` and close the body with `::grepple end TOKEN`. While a body is open, every other line—including `::grepple file`, quotes, backticks, and dollar signs—is literal source. Blank lines outside bodies are ignored. One body line maps to one `content_lines` entry; include a blank line before a create terminator to preserve a final newline in the new file.

The heredoc envelope and strict JSON both compile to the same in-memory `grepple-write-v1` request. File/change order, original-snapshot anchors, empty deletion content, size limits, confinement, stale checks, overlap rejection, mixed edit/create/delete atomicity, dry runs, output modes, staging, concurrent-mutation checks, and rollback are identical.

## Literal single-edit input

For one edit, avoid JSON and shell escaping by reading the replacement as literal text:

```sh
grepple write edit --root . --path path/to/file.kt --start abc --end def <<'EOF'
val payload = """{"labels":["owner"]}"""
EOF
```

`--end` defaults to the `--start` anchor for a one-line replacement. Use `--content-file PATH` to read the replacement from a file, or `--content-file -` (the default) to read stdin. One terminal LF or CRLF terminates the final replacement line and does not create an extra blank line; use two terminal newlines to retain one blank line. Empty input deletes the anchored range.

Literal single-edit mode builds the same in-memory `grepple-write-v1` edit and uses the same confinement, stale-anchor, size, newline, transactional installation, output, and dry-run behavior. Use either the literal multi-file envelope or strict JSON when multiple edits or files must be committed atomically.

A successful human response returns the resulting edit-ready rows:

```text
path/to/file.go

Ab3│42│func updated() {
K9x│43│    run()
Pq2│44│}

applied 1 files, 1 changes
```

Fresh anchors are recomputed against the complete resulting file and are associated with `after_sha256` in `--json` output. Replacement rows include neighboring anchors for orientation. Range deletions return surviving neighbors.

Successful, non-dry-run human writes also add those exact post-write rows to Grepple's session context coverage. If prior structural output plus the write anchors cover every current line of a declaration, a later structural search may replace the declaration with an unchanged-source marker. Partial coverage never suppresses a declaration. JSON, predicted dry-run anchors, failed writes, and output that spills are not recorded. Use `--repeat-source` on the later structural search to force complete source and increment the context guard's `bypassRequestedCalls` statistic.

## Dry runs and structured output

`--dry-run` performs all validation without mutation. Human output includes deterministic unified diffs followed by predicted anchors:

```text
grepple write --root . --dry-run < request.json
```

Use `--json` when a script needs the complete `grepple-write-v1` response:

```text
grepple write --root . --json < request.json
grepple write --root . --dry-run --json < request.json
```

JSON includes operation, before/after SHA-256 digests, unified diffs for dry-run inspection, request-ordered change details, resulting line ranges, deletion markers, and anchors. The default changed from JSON to edit-ready human output before a stable release; automation must pass `--json` explicitly. Large complete responses use the normal content-addressed spill mechanism unless `--no-spill` is requested.

## Creating files

Creation is explicit and only targets an absent repository-relative path beneath an existing confined directory:

```json
{
  "path": "path/to/new.go",
  "operation": "create",
  "content_lines": ["package sample", ""]
}
```

The equivalent literal operation is `::grepple file path/to/new.go` followed by `::grepple create`, its body, and `::grepple end`.

Each array entry is one logical line. A trailing empty entry preserves a final newline. Created files use mode `0644`, subject to platform support, and participate in the same transaction and rollback as edits and deletions. Existing targets, symlink targets, missing parents, directory escapes, embedded newlines, NUL bytes, and oversized results are rejected.

## Deleting files

Deletion requires the exact digest of the current file and does not accept changes or file content:

```json
{
  "path": "path/to/old.go",
  "operation": "delete",
  "before_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
```

The equivalent literal operation is `::grepple file path/to/old.go` followed by `::grepple delete SHA256`.

A stale digest rejects the entire transaction. Deletions are installed through the same backup sequence as replacements and are restored if a later installation fails.

## Transaction contract

Before mutation, Grepple validates every operation, path, filesystem identity, file type, digest, anchor, range, replacement line, overlap, newline style, and size limit. Duplicate lexical paths and hardlink identities are rejected. Changed content is staged beside its destination, existing bytes are rechecked for concurrent changes, creates are installed without replacing a target that appeared concurrently, and existing files are backed up before installation.

A rejection changes no requested file and exits with status 1. Human errors include concise path/change context and fresh nearby anchor rows when available. `--json` returns a structured error with the same evidence. Rollback failures remain visible in the error rather than being discarded.

As with any multi-file operation on ordinary filesystems, an operating-system crash or power loss during the final installation sequence is not a general filesystem transaction. Grepple provides complete prevalidation and best-effort rollback, not a cross-filesystem journal.

## Safety and limits

- `--root` defaults to the working directory.
- Request paths must be relative and resolve inside the root.
- Existing file and directory symlink escapes, directories, non-regular files, and missing edit/delete targets are refused.
- Create requires an absent target and an existing confined parent directory.
- Delete requires a lowercase 64-character SHA-256 digest.
- Existing LF or CRLF style and final-newline behavior are preserved by edits.
- Mixed or bare-CR source files and NUL-containing existing files are refused.
- Content entries cannot contain embedded newline characters.
- Request size is capped at 16 MiB; each source or result file is capped at 16 MiB.
- A transaction supports at most 128 files and 4,096 operations/changes.
- No shell, configured anchor provider, or model is executed by `grepple write`.
