# Navigation latency diagnosis

## Different measurement scopes

The earlier roughly 190x result timed only a tiny depth-2 synthetic neighborhood
on an already-built/open immutable index. It excluded process startup, corpus
file discovery/reads, source/context revision validation, index reopening and
root selection. It does not predict the complete CLI command's speedup.

Re-running the 200-sample synthetic test currently gives:

| Query-only mode | p50 | p95 |
| --- | ---: | ---: |
| Reference | 2.227 ms | 4.358 ms |
| Resident compact | 10.826 us | 18.025 us |
| ReadAt compact | 16.239 us | 28.931 us |

The query-only improvement remains real: resident ~206x p50, ReadAt ~137x p50.
Historical sample ratios vary; do not equate these with CLI-wide improvements.

## Root-selection bottleneck found and fixed

Stage timing of `--at` on the current stored views found root selection spending
3.4-3.5 seconds in grepple (6841 declarations), ~0.86-0.96 seconds in backend
(1795), and ~0.11 seconds in dashboard (223).

`selectSingleGraphRoot` called `cleanPath` twice per declaration. Each cleanPath
used `filepath.Abs` on the path and on `.`; relative paths repeatedly invoked
os.Getwd and its filesystem checks. CPU profiles show filepath.Abs/Getwd/syscalls
on that hot path; wall-stage timers capture the much larger wait cost.

Fixed by capturing cwd once per selection and normalizing the requested path
once, then using lexical absolute/relative conversion for declaration paths.
No process-global cwd cache; directory changes between commands remain supported.
The same redundant cwd work in ancestor-context collection was removed. Content
hashes, missing-manifest checks, complete selected source sets and policy filters
are retained; no mtime-only shortcut or relaxed revision fence was introduced.

After the root fix: grepple ~2.3-3.8 ms, backend ~1.0-1.2 ms, dashboard ~0.35-0.72 ms
for root selection. Path-normalization parity, cwd-change and existing selection/
ambiguity/filter tests pass under race detection.

## Other stages

Representative pre-context-collection-fix warm diagnostic stages:

| Stage | grepple | backend | dashboard |
| --- | ---: | ---: | ---: |
| Discovery | 281 ms | 189 ms | 85 ms |
| Source reads | 1325 ms | 804 ms | 189 ms |
| Source/config fence | 632-717 ms | 501-549 ms | 223-233 ms |
| Open core/manifest | 28-33 ms | 8-9 ms | ~2 ms |
| Declaration hydration | ~9 ms | ~2 ms | <1 ms |
| Traversal + selected details | ~9-10 ms | ~3-4 ms | <1 ms |
| Report JSON | ~1-3 ms | <1 ms | <1 ms |

These are individually timed stages, not additive controlled medians across
identical disk conditions. CPU profiling is not a wall-wait attribution tool.
Source reads and validation now dominate; core reload and declaration hydration
are not the multi-second culprit.

## Real query-only comparison

On the same decoded reference graph versus its pinned stored generation, without
root lookup or opening costs, the measured three-sample medians were:

| Corpus | Reference | Stored | Returned declarations/calls |
| --- | ---: | ---: | ---: |
| grepple | 9.633 ms | 4.515 ms | 79 / 103 |
| backend | 2.313 ms | 1.128 ms | 38 / 23 |
| dashboard | 174 us | 96 us | 1 / 0 |

Real neighborhood projections carry native context/type/import/usage details;
the grepple report is ~576 KB, not the synthetic fixture's tiny neighborhood.
Selected hydration limits the improvement here to roughly 2x, not 190x. Semantic
parity passes; three samples do not establish p95. Full Graph hydration happened
outside these query-only timers and is not part of the warm CLI path.

## Final end-to-end comparison

Same final binary and unchanged corpus within each pair, three warm samples,
GOMAXPROCS=2 / GOMEMLIMIT=384MiB, `--no-spill graph callers --at ... --depth 2 --json .`:

| Corpus | Reference with root fix | Stored with fixes |
| --- | ---: | ---: |
| grepple | 3.618 s | 2.113 s |
| backend | 1.486 s | 1.528 s |
| dashboard | 0.517 s | 0.661 s |

Earlier stored end-to-end values were 6.915 / 3.051 / 0.956 seconds, but source
files/binaries changed during implementation. The stage before/after comparisons
use the same frozen path lists. Do not present cross-binary totals as a controlled
speedup. The final paired comparison passes semantic/source/query JSON parity
(excluding renderer metadata). Fresh-process source fencing still prevents an
across-the-board CLI latency win on smaller corpora.

## Reproduction and next work

Opt-in test-only diagnostics: `TestNavigationDiscoveryLatencyDiagnostic` and
`TestNavigationStoredLatencyDiagnostic`. Set GREPPLE_NAVIGATION_DIAGNOSTIC_ROOT,
GREPPLE_NAVIGATION_DIAGNOSTIC_OUTPUT, GREPPLE_NAVIGATION_DIAGNOSTIC_AT and a separate
GREPPLE_NAVIGATION_CACHE_DIR. Run discovery first; it writes the frozen paths list.

Artifacts: `/workspace/.tmp/nav-latency-diagnostic/` contains per-corpus stage logs,
CPU profiles, paired CLI outputs/results and the synthetic 200-sample log.
Initial diagnostic Open misses came from using a nil filter instead of the
normalized empty filter; the harness was corrected, not the production cache.

Next: reduce repeated source/config I/O with a revision-safe validated generation
strategy, profile first-build conversion separately, and avoid optimizing tiny
root/declaration costs at the expense of the actual dominant work. No forced cache
flush, production deployment, installation, backend change, commit or push.
