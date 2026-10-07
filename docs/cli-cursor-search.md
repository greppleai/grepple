# CLI snapshot cursor search

Use `grepple --remote-only -F Connect --related` for snapshot-bound Public content
search without local results. JSON metadata and stderr provide a copyable next
command containing `--cursor` and the same server. Repeat that command to advance;
rerunning an identical cursor command replays the same page. Only completion—not
an empty result page—means exhaustion. Cursors require the same fresh account
and expire with the search session. An error never silently falls back to legacy.

`--remote` / `--server` still preserve local-plus-remote behavior. Content searches
use cursor pages when there are no local matches. Mixed local/remote windows,
offset links, exact-source lookup, file listing, counts, match sorting and
`--max-files` retain the legacy endpoint for compatibility. `--remote-only` is the
explicit way to guarantee cursor traversal. Local search is unchanged.

Snapshot traversal is unordered globally; the CLI sorts within each returned
page for presentation. No complete result count is claimed. Pages are at most
100 files and a replay is indivisible: lowering `--limit` on continuation does
not drop files already selected into that frozen page. Change page size or query
by starting a fresh search instead. Match lines and related context remain pinned
to the source snapshot; navigation hints are not resolved against mutable files.

The existing authenticated, bounded, nonredirecting HTTP transport is reused.
Page requests have a 40-second deadline, and the server continues checking fresh
Public authority on every initial request, continuation and replay.
