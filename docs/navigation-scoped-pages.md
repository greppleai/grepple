# Scoped navigation pages: CLI-first shared storage

The shared `internal/navigation` SnapshotStore now exposes SavePaged/OpenPaged.
CLI language partitions use it by default in source. Neither the installed CLI nor
backend wiring/deployment was changed. Existing backend GNSTO001 files stay intact.
No commit/push or historical generation cleanup was performed.

## Storage and query path

GNSTO004 files reuse immutable temp/sync/rename/directory-sync publication and
pinned-file ownership. GNPDIR01 is a checked binary directory rather than a JSON
array of decimal SHA bytes. It records independent section offsets, lengths and
SHA-256 digests. Open verifies the directory and all bounds, but reads **zero core
pages**. No full-core checksum sweep precedes a targeted query.

The resolved graph remains authoritative. Every fact is persisted; resolution and
build semantics have not been changed or expanded. Within a language-connected
partition, numeric call/target/group and CSR offset/posting sections have 4096
uint32 values per page. Name/ID and sorted key-locator sections use 128 entries per
page, avoiding the measured overfetch from initial 4096-entry string pages. Few
segment files contain all pages; no OS file per symbol/package/page.

The reader resolves an exact stable ID via sorted locator pages, reads only the
relevant reverse/forward posting ranges, batches frontiers by numeric row, then
hydrates selected declaration/call/context native detail blocks. It preserves
candidate order, duplicates, ambiguity, confidence, unresolved endpoints, field
owner lookup and native detail nil/empty shapes. Cross-scope edges stay in the
same global numeric identity universe, not guessed per-package graphs. Language
partition merging and selective foreign field-context copies retain the previous
language-split semantics, including embedded-language coordinates.

CLI root selection still uses a global slim catalog for exact ambiguity and
narrowest --at behavior. Version-3 manifests carry GNROOT01 checked binary catalog
bytes; source/package identities are shared. This is not yet a paged qualified-
name/FST root selector. Multi-language independent-root requests deliberately use
the existing complete-view fallback; full queries never silently lose partitions.

## Reader budgets and remaining admission work

Each query has a 4 MiB encoded-page cache plus an estimated 4 MiB decoded locator
cache. There is no retained full adjacency graph or shared unbounded page cache.
Explicit limits: 128 MiB cumulative core reads, one-million numeric range/selection
limits, and two simultaneous paged traversals per process. Budget errors have a
sentinel and CLI does **not** bypass them through an unbounded rich rebuild.
Corruption is independently checked on access; untouched damaged pages do not
activate, but full projections fail when they reach them.

These are reader limits, **not a guaranteed total-cgroup memory reservation**:
native returned facts, compressed detail expansion, opened directories, source
buffers and cold builds need separate accounting. Admission waits currently have
no context-aware cancellation; returned output outlives the scratch reservation.
Backend reuse must add request cancellation and real byte reservations before
claiming worst-case concurrent cgroup safety. Large-result streaming/spill is not
implemented here; over-budget traversals return errors, never partial success.

Cold building still resolves the full universe before partitioning its output.
Conversion peaks are not solved by selective reopening. Physical pages are not
new compiler/build scopes; no invented cfg, build-target or Python-environment
bindings. Semantic configuration fences remain the existing exact source/config/
executable/filter/max-files inputs, not mtime/HEAD-only shortcuts.

## Measurement: current mixed checkout

Same binary/corpus/request pairs, GOMAXPROCS=1, GOMEMLIMIT=256MiB, ordinary warm
filesystem behavior. All three Go/Python/shell paired JSON outputs matched except
renderer metadata. Twenty isolated reader samples include reopen and query; their
medians are not production p95/cold-cache claims. GC only occurs inside measurement
tests, never production query paths.

Compared with the preceding eager **Go partition**:

| Reader workload | Eager → paged median | Allocated bytes eager → paged |
| --- | ---: | ---: |
| Small languageManifestPartition callee query | 38.68 → 4.26 ms | 9100994 → 1781930 |
| Broader BuildStoredGraph callee depth 2 | 49.96 → 17.85 ms | 12556222 → 6146785 |
| Explicit complete Go graph projection | 202.85 → 184.02 ms | 138471240 → 136201770 |

Small query: 144128 core bytes / 10 pages plus 306082 directory bytes. Broader:
595430 core bytes / 52 pages. These are actual ReadAt core reads, not physical
storage I/O/fault counts; native detail bytes are not included. Complete projection
still hydrates every native result, so its allocation gain is only modest.

Including CLI global root selection/catalog loading, Go reader median was
69.85 → 39.37 ms, allocated bytes 21885943 → 15493003. Python/shell partitions are
already tiny: paging alone showed no convincing additional latency benefit.

Whole-CLI three-warm-sample medians:

| Root | Complete repository core | Eager language partition | Paged language partition |
| --- | ---: | ---: | ---: |
| Python | 2.128 s | 2.178 s | 2.112 s |
| Shell | 2.163 s | 2.161 s | 2.183 s |
| Go | 2.207 s | 2.188 s | 2.111 s |

Paged peak RSS medians were 85404/85088/94632 KiB for Python/shell/Go, versus
98116/97728/103572 KiB with the complete view. RSS is not retained heap. No 9x
whole-CLI claim: source discovery, bytes and revision fences still dominate.

Go file: eager 15019221 B, paged 15034496 B; numeric/name/context core
2428512 → 3180815 B, directory 1043058 → 306030 B. Duplicate locators/dense offset
ranges trade core bytes for selective access; the binary directory offsets most
of that increase. The separate CLI root manifest is 1261514 B. Total generation
storage has not been shown to shrink. Historical and experimental generations
remain preserved and are excluded from per-view comparisons.

First missing-generation requests were 13.24/13.30/13.83 s for complete/eager/paged,
RSS 537944/523788/521364 KiB. They are sequential warm-filesystem builds with
independent parser caches, not cold-cache measurements or a build-peak solution.

Artifacts: /workspace/.tmp/cli-scoped-navigation/{results-final.json,
sizes-final.json,final-*-load.log,*-reader.log,*-reader.time,grepple-final}.
Measurement excludes source discovery in reader tables and includes it in CLI
tables; do not conflate these scopes.

## Validation and next steps

Full public tests; navigation/analysis/CLI/API races; schema check; focused lint;
all 18 adapters, randomized and rich graph parity across five directions/depths,
candidates/duplicate IDs/empty shapes, complete projection, coherent language
invalidation, concurrent publication/readers, pinned replacement, selective
corruption, binary truncation/trailing/count guards and explicit budget errors.

Next: paged root selectors rather than loading the whole slim catalog; strict
context-aware byte admission and native detail/output budgets; paged generation
multi-root traversal; measured larger mixed corpus/Codex Python and Rust workloads;
streamed cold construction. Backend integration remains a separate step.
