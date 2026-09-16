# Transactional hashline writes

`grepple write` applies one validated request across multiple existing files. It uses Grepple's built-in `hashline-v1` implementation, so anchors emitted by `grepple --anchors` can be sent back without an editor-specific adapter.

## Workflow

Retrieve edit-ready rows:

```text
grepple --anchors --line-only -F 'old value' path/to/file.go
```

Anchored rows are exactly:

```text
HASH│LINE│content
```

Submit one strict JSON request on stdin:

```sh
grepple write --root . <<'JSON'
{
  "schema": "grepple-write-v1",
  "files": [
    {
      "path": "path/to/first.go",
      "changes": [
        {
          "hash_range_inclusive": ["abc", "def"],
          "content_lines": ["replacement", "lines"]
        }
      ]
    },
    {
      "path": "path/to/second.go",
      "changes": [
        {
          "hash_range_inclusive": ["ghi", "ghi"],
          "content_lines": []
        }
      ]
    }
  ]
}
JSON
```

Each range is inclusive. `content_lines: []` deletes the range. To insert around an existing line, replace that line with the original line plus the inserted lines. Every range in a file refers to the same original snapshot, so one request can safely contain multiple non-overlapping changes in any order.

Use `--dry-run` to perform all parsing, path, file, newline, hash, range, overlap, and size validation without changing files:

```text
grepple write --root . --dry-run < request.json
```

## Transaction contract

Before writing, Grepple validates every file and every change. If any path or anchor is invalid, no requested file is changed. Changed files are staged beside their originals, permissions are preserved, source bytes are rechecked for concurrent changes, and installation uses backups with rollback on normal rename failures.

The command returns a bounded `grepple-write-v1` JSON response. Success includes per-file before/after SHA-256 digests and change counts. Rejections return exit status 1 and a structured error. A `stale_anchor` error includes up to five current nearby `HASH`, line, and content rows for recovery.

As with any multi-file operation on ordinary filesystems, an operating-system crash or power loss during the final rename sequence is not a general filesystem transaction. Grepple provides prevalidation and best-effort rollback, not a cross-filesystem journal.

## Safety and limits

- `--root` defaults to the working directory.
- Request paths must be repository-relative and resolve inside the root.
- Existing symlink files, escaping directory symlinks, directories, missing files, and creates are refused.
- NUL-containing files are refused.
- LF and CRLF are preserved; mixed or bare-CR newline files are refused.
- Replacement entries cannot contain embedded newline characters.
- Overlapping ranges and duplicate resolved file paths are refused.
- Request size is capped at 16 MiB.
- Each source file is capped at 16 MiB.
- A transaction supports at most 128 files and 4,096 changes.
- No shell or configured anchor provider is executed by `grepple write`.

Anchors are intentionally short and file-local. Duplicate line content receives deterministic collision retries, making every anchor unique within its current file snapshot. Anchors must be refreshed after a successful edit.
