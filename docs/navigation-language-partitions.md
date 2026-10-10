# CLI language-partitioned navigation

This document records the earlier language-only layout and measurements.
Current default CLI storage uses scoped pages; see [navigation-scoped-pages.md](navigation-scoped-pages.md).
The eager layout remains available through GREPPLE_NAVIGATION_EAGER_PARTITIONS=1.
Local CLI graph queries use language-partitioned persisted views in source.
The installed CLI and deployed backend were not replaced for this experiment.
Use GREPPLE_NAVIGATION_MONOLITHIC=1 for same-binary comparison with the preceding
complete-core implementation; GREPPLE_NAVIGATION_REFERENCE_GRAPH=1 remains the
fresh rich-graph parity path.

## Layout and semantics

Each source/config/executable/filter/max-files-fenced generation publishes:
- `<key>.gnav.languages.json`: checked version-2 manifest and slim root catalog.
- `<key>.gnav.languages-<unique>/N.gnav`: immutable independently checked cores
  and detail tables, identified by language in the manifest.

All language facts are stored, including unqueried languages. Root source/package
identity is dictionary-shared across callable rows. Unqualified selectors retain
global ambiguity detection; narrowest --at selection and output order are unchanged.

Partitioning follows filtering, never replaces it. Ordinary Python queries open
Python, not Go/Rust cores. Actual call/candidate/identity links and shared physical
file context conservatively connect languages; e.g. native Svelte/CSS/TypeScript
context remains together. No compiler/FFI bindings are guessed. Current globally
keyed field-owner lookup semantics are preserved by copying the applicable field
and source-context detail facts, not by connecting entire foreign adjacency cores.

Roots spanning multiple independent partitions deliberately use the complete-view
fallback for now. This preserves global ordering/duplicates without implementing
an unsafe output-only merge. A cold multi-partition request can prepare both views;
subsequent requests reuse the complete cached view. This is not yet a fully lazy
cross-partition multi-root traversal engine.

Source content, config presence/content and executable fences still cover every
input language, even for single-language queries. No mtime/Git-HEAD shortcuts.
Publication writes/syncs every immutable partition before atomically replacing
and syncing the manifest pointer. Readers own pinned snapshot files. Failed
publication falls back to fresh computation; incomplete generations are invisible.
Old generations, protobuf history and rollback data are not automatically deleted.
Selected corruption rebuilds the generation; unrelated partition corruption does
not activate/rebuild it on a Python query.

## Measured mixed checkout

Corpus: current /workspace/grepple. Same binary/corpus/query pairs, separate modes,
normal filesystem cache, GOMAXPROCS=2 and GOMEMLIMIT=384MiB. Three warm real CLI
samples are medians, not p95/cold-cache measurements. Each paired JSON report
matched excluding renderer metadata.

| Root | Whole CLI complete → split | Peak RSS median complete → split |
| --- | ---: | ---: |
| Python benchmark run | 2.120 → 2.039 s | 98540 → 86024 KiB |
| Shell fetch | 1.998 → 2.114 s | 98168 → 85140 KiB |
| Go BuildStoredGraph | 2.062 → 2.015 s | 102252 → 99956 KiB |

No convincing whole-CLI latency improvement: discovery, source reads and revision
fences dominate. Shell fluctuated/regressed; Go is mostly the corpus already.
Sequential first builds reuse parser caches, so their times are not fair cold-build
comparisons and are not claimed as a build improvement.

Twenty isolated filesystem-warm reader samples, including manifest/core open,
root selection, detail hydration and query but excluding discovery/source fences:

| Root | Reader median complete → split | Mean allocated bytes complete → split |
| --- | ---: | ---: |
| Python | 51.70 → 24.97 ms | 22280048 → 9611655 |
| Shell | 52.18 → 22.24 ms | 21971390 → 9345320 |
| Go | 60.73 → 66.08 ms | 25582606 → 21413790 |

The Go reader regressed ~9% despite lower allocations; the shared root directory
has overhead and remains a target for indexed/paged lookup. GC is restricted to
measurement tests, not production query paths.

Disk: complete view 15202654 B; split snapshot files total 15224410 B plus root
manifest 1158808 B (~7.8% more total storage). Python selected view plus manifest:
1264599 B (~91.7% smaller than complete); shell: 1164437 B (~92.3% smaller).
These are selected storage footprints, not exact physical read-byte counters;
core checksums and detail selection can reread or skip sections. Rich history and
unused experimental generations are excluded from per-view size comparisons.

Artifacts: /workspace/.tmp/cli-language-partitions/{results.json,sizes.json,
comparison-final.log,*-load.log}. Test binary available there; no installation.

## Validation and next work

Full tests, navigation/analysis/CLI/API races, schema check and focused lint.
Fixtures cover all 18 adapters, five directions, mixed Go/Python/shell roots,
multi-root fallback, cross-language candidates/duplicates, foreign field context,
embedded language context, coherent invalidation, corruption and concurrent publish.

Next: cheaper indexed root lookup and coherent cross-partition multi-root traversal;
then apply language-owned storage/reader admission to backend with real Python-in-
Codex measurements. Existing deployed Codex snapshots remain unchanged.
