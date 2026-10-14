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
