# CLI stateless cursor search

Use `grepple --remote-only -F Connect --no-related` for Public content search
without local results. JSON metadata and stderr provide a copyable next command
containing `--cursor`, the same server, query, filters and page size. Every page
resends those options. Changing them invalidates the cursor: start a new search.

Tokens contain a query/options digest and global skip offset, not credentials,
account identity, expiry or snapshot references. Fresh authorization applies on
every request. Any router replica can execute a continuation without disk state
or affinity. Index changes may repeat or omit files. Replaying a command searches
the current index at that offset; it does not return a frozen saved page.

`--remote` / `--server` retain local-plus-remote behavior. Mixed local/remote,
explicit offsets, exact-source, listings, counts, match sorting and `--max-files`
retain legacy endpoints. Local search is unchanged. Cursor pages use global path
ordering and do not claim a complete result count. Normal page limits apply;
offsets are bounded at 10,000 files and deep pages can be slower. A full page can
have an empty final continuation. Errors never silently fall back to legacy.

The authenticated, bounded, nonredirecting HTTP transport is reused with a
40-second deadline. Old opaque session cursors are invalid; submit a new search.

## Legacy offset pages

For modes that do not support cursors, use `--remote-only --skip N --limit N`.
The server applies the offset before the per-page cap of 100 files; the CLI does
not skip those results again. For example, to list the second remote page:

```sh
grepple search --remote-only --server http://localhost:8080 -F needle --files-with-matches --limit 100 --skip 100 --json
```

`--limit 0` and limits over 100 are clamped to 100 when remote search is enabled.
`--max-files` is an additional per-page cap, not a cap on the prefix before the
offset. Legacy responses do not establish a complete remote corpus count; a
short page alone is not proof of complete coverage. Check truncation/shard warnings.

Mixed local/remote searches fetch a prefix from each source before applying one
merged window. They require `skip + effective limit <= 100`, where the effective
limit is the smaller positive value of `--limit` and `--max-files` after remote
limit clamping. Requests exceeding that prefix cap fail explicitly, even if no
local files happen to match. Split these searches with `--remote-only` and
`--local` rather than treating a capped prefix as exhaustion. Eligible remote
content searches can instead follow their emitted cursor continuation commands.
