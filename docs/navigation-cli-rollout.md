# CLI-first navigation rollout checkpoint

Status: local CLI graph **queries** now use persisted compact filtered views in
source. Warm queries bypass NewUniverse parsing and rich graph deserialization.
Installed CLI, related/source navigation and backend have not been switched.
No runtime deployment, installation, commit or push performed here.

## Current-checkout CLI baseline

Built current source into `/workspace/.tmp/nav-cli-current-baseline/grepple`.
Compared reference vs experimental compact traversal in the same binary with
`--no-spill graph callers --depth 2 --json .`, three warmed runs per mode,
GOMAXPROCS=2 and GOMEMLIMIT=384MiB. These are medians, not p95 estimates.

| Checkout / selector | Reference | Compact traversal only | Selected / parsed |
| --- | ---: | ---: | ---: |
| grepple / internal/navigation/services.go:68 | 6.469 s | 6.524 s | 864 / 864 |
| backend / internal/search/service/search.go:35 | 2.076 s | 2.255 s | 373 / 373 |
| dashboard / src/lib/search.ts:95 | 0.602 s | 0.577 s | 122 / 122 |

JSON semantic/source/query parity passes for these cases, excluding renderer
metadata. Returned declarations/calls: 69/88, 38/23 and 1/0. No failed sources;
recovered counts were 4, 0, 1. Outputs, timing/RSS logs and comparisons are in
`/workspace/.tmp/nav-cli-current-baseline`. The initial runs without `--no-spill`
produced artifact references for larger outputs; their zero array counts are
not semantic graph counts and are not used in this comparison.

The earlier traversal-only comparison did not improve end-to-end CLI latency.
It retained the complete parsing/loading path. The subsequent persisted reader
removes that path on a valid cache hit, but source discovery, byte hashing,
context checks, declaration root selection and executable startup still run.

## Persisted CLI comparison

A second same-binary comparison used `grepple-stored` and separate per-repository
cache directories. Each repository was unchanged between its stored and reference
runs. Three warm samples per mode; first stored build reported separately. These
are filesystem-warm measurements, not controlled cold-cache tests or p95 claims.
Reference used `GREPPLE_NAVIGATION_REFERENCE_GRAPH=1`; compact traversal's older
experimental switch was unset. Runtime limits remained GOMAXPROCS=2 / 384MiB.

| Checkout | Reference median | Stored warm median | First stored build | Reference / stored warm RSS |
| --- | ---: | ---: | ---: | ---: |
| grepple | 8.757 s | 6.915 s | 31.741 s | 420328 / 107492 KiB |
| backend | 2.905 s | 3.051 s | 10.900 s | 221308 / 134748 KiB |
| dashboard | 0.784 s | 0.956 s | 3.690 s | 41412 / 29508 KiB |

Warm latency: grepple about 21% faster, backend about 5% slower, dashboard about
22% slower. Warm process peak RSS about 74%, 39%, 29% lower respectively. First
build RSS: 530644 / 241740 / 44496 KiB. Conversion/build peak remains worse than
reference; this is not a solved cold-start performance problem.

Semantic/query/source JSON parity passes for all three, excluding renderer
metadata. Selected/parsed: 871/871, 373/373, 122/122. Source counts changed from
the earlier baseline as implementation files were added; do not compare these
numbers directly to the previous binary/corpus measurements.

Stored view sizes: 14957424 / 3801445 / 282136 bytes. These filtered view sizes
are not full-fact artifact storage comparisons. The grepple core is 2419313
bytes and its manifest 1041216 bytes; detail payload reads remain selective.
Strace of a warm backend query shows substantial filesystem/stat and runtime
wait overhead, only 379 pread calls; no exact wall-time attribution claimed.
Results/logs: `/workspace/.tmp/nav-cli-current-baseline/stored-comparison.json`,
`stored-*.time`, `stored-*.json`; diagnostic `/tmp/nav-stored-strace.txt`.

## Implemented

- Explicit deterministic little-endian core codec and traversal range validation.
- `GNSTO001` immutable generation file: fixed header, checksummed manifest/core,
  eight addressable native detail tables with checked offsets/lengths/digests.
- Temporary file, sync, atomic rename and directory sync publication; pinned open
  inode through traversal/hydration, Close synchronized with active queries.
- Local CLI filtered views keyed by executable, cwd, all selected source bytes,
  normalized filter, max-files and bounded ancestor configuration manifests,
  including absent manifests. Context fence rechecked before publication.
- Filtering before traversal exactly as before. Different roots/directions share
  a stored view; different filters are separate views. Original full facts and
  historical protobufs are retained through the existing build/cache path.
- Warm path loads numeric core and declaration details for existing root-selection
  semantics, not the complete graph or source documents. Other details use ReadAt.
- Corruption causes a fresh rebuild; failed sources are not published as reusable
  views. Source changes cannot reuse an old view merely because Git HEAD matches.
- Determinism, full graph parity, directional cold/warm parity, source/context/
  filter/limit invalidation, corruption, concurrency and replacement-lifetime tests.

## Remaining gates

1. Stream cold detail encoding directly to storage; avoid resident encoded blocks
   alongside the rich graph/documents. Reduce first-build latency and peak RSS.
2. Replace all-declaration hydration for root selection with indexed slim metadata;
   reduce source/config validation and discovery overhead without trusting mtimes
   alone or weakening dirty-checkout fences. Measure larger samples/cold reloads.
3. Harden/report storage failure and serialized resource-bounded rebuild admission;
   investigate paged hot arrays and a process-wide byte-bounded cache.
4. Expose the reader through the public API and integrate backend artifact ownership
   and selected-file navigation. Preserve exact dependency qualification evidence,
   proactive all-outdated builds, stateless pagination, native Zoekt ownership and
   all immutable architecture policies. Codex discovery remains separately blocked.

Historical protobufs, backups, rollback images and unrelated work remain intact.
Incremental resolution and PostgreSQL catalog changes are out of this rollout.
