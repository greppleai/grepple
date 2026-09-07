# Markdown & plain-text search

Non-code text is searched line-by-line. Markdown gets its enclosing heading chain, and
everything else (logs, prose, config without an outline grammar) falls back to plain
matching lines.

## Markdown heading context

The matched line is shown beneath the `#`…`######` breadcrumb that contains it:

```bash
grepple "minVersion" sample-files/guide.md
```

```text
sample-files/guide.md

 1   # Search Guide

// … 3 lines collapsed …

 5   ## Configuration

// … 3 lines collapsed …

 9   ### TLS

// … 1 line collapsed …

11   Enable TLS in production. The `minVersion` should be at least 1.2.
```

## Plain text / logs

A log file has no grammar, so matches print as plain lines with the surrounding lines
collapsed:

```bash
grepple "ERROR" sample-files/server.log
```

```text
sample-files/server.log


// … 3 lines collapsed …

4   2026-08-31T09:16:03Z ERROR upstream timeout path=/index status=504
```

### Context lines (`-C`)

Use `-C N` for grep-style context. This switches to the `path:line:` / `path-line-`
prefix format (`:` marks the match, `-` marks context):

```bash
grepple -C 1 "ERROR" sample-files/server.log
```

```text
sample-files/server.log-3-2026-08-31T09:15:42Z WARN  slow request path=/search duration=812ms
sample-files/server.log:4:2026-08-31T09:16:03Z ERROR upstream timeout path=/index status=504
sample-files/server.log-5-2026-08-31T09:16:03Z INFO  retrying request path=/index attempt=2
```

### Just the matching lines (`--line-only`)

Skip structural rendering entirely and print `path:line:text`:

```bash
grepple --line-only "INFO" sample-files/server.log
```

```text
sample-files/server.log:1:2026-08-31T09:15:01Z INFO  server starting addr=0.0.0.0:8080
sample-files/server.log:2:2026-08-31T09:15:01Z INFO  loaded 12 routes
sample-files/server.log:5:2026-08-31T09:16:03Z INFO  retrying request path=/index attempt=2
sample-files/server.log:6:2026-08-31T09:16:04Z INFO  request ok path=/index status=200
```
