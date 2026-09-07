# JSON output & paging

Machine-readable output for tooling, plus how to page through large result sets.

## Compact JSON matches (`--json-matches`)

`--json-matches` returns just the matching lines as structured data:

```bash
grepple --json-matches "ERROR" sample-files/server.log
```

```json
{
  "matches": [
    {
      "line": 4,
      "path": "sample-files/server.log",
      "text": "2026-08-31T09:16:03Z ERROR upstream timeout path=/index status=504"
    }
  ]
}
```

Use `--json` for the full structural result set (files, segments, and metadata).

## Paging (`--limit` / `--skip`)

Results are returned in **deterministic path order** and paged by **file**. The default
`--limit` is 20 (`--limit 0` = all). Combine with `--files-with-matches` for a compact
page listing.

Page 1 — the first two matching files:

```bash
grepple -i "server" sample-files --limit 2 --files-with-matches
```

```text
sample-files/config.json
sample-files/guide.md
```

Page 2 — skip those two and take the next two:

```bash
grepple -i "server" sample-files --skip 2 --limit 2 --files-with-matches
```

```text
sample-files/server.go
sample-files/server.log
```

Because ordering is stable (there is no relevance ranking), the same `--skip`/`--limit`
window always returns the same files — so you refine by narrowing (tighter pattern,
globs, `--limit`) rather than trusting a score.
