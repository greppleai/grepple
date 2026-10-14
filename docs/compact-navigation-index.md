# Compact navigation query index

Status: experimental Phase 1/2 core and Phase 3 ReadAt detail-reader investigation;
not a completed backend rollout. See [storage investigation](navigation-snapshot-storage.md)
for measured disk-backed detail retrieval and the remaining direct-reload design.
Specification: `/workspace/nav-idx.md`. Tree-sitter extraction, semantic resolution,
Zoekt, artifact/download schemas, source selection and external confidence labels
are unchanged. No historical cache or deployed index is deleted.

## Audit and lifetime matrix

| Facts | Resolver/build consumers | Query/render consumers | Snapshot placement |
| --- | --- | --- | --- |
| Callable declarations, stable IDs, file/range, owner, signatures | `internal/navigation/engine.go`, language indexes | root selection in `internal/analysis/graph.go`; graph queries, filters, diff and directory architecture | ID-to-numeric-node dictionary and source-row indexes; rich native detail records |
| Calls, direct/candidate targets | `engine.go` receiver/import/language resolution | `query.go` all five directions; `stats.go`, graph/architecture output, search related context | fixed-width numeric records, contiguous target table, outgoing/incoming CSR; every original rich call retained |
| Confidence and inference fields | resolver and external evidence qualification | confidence filters, statistics, explanations and graph output | original classifications retained in native details; never promote heuristic or candidate edges |
| Type declarations | language/type resolution | `internal/analysis/graph.go`, architecture outlines, search type context | file-to-original-type-row index and rich detail records; row identities do not merge same-name types |
| Fields, embedded fields | `engine.go` receiver and promoted-type inference | context projection; full architecture/type output | owner-to-source-row index; complete field/type/embedding facts in native details |
| Type usages and roles | parser/type resolution | callable-owned projection, graph/architecture output, search type context | numeric caller-to-row indexes and complete native details |
| Member accesses, operation and location | parser facts | callable-owned projection and graph output | numeric caller-to-row index and complete native details, not dropped as unused |
| Imports and exports/re-exports | language indexes and external dependency qualification | file context projection; graph filters/full output | file-to-row indexes and native detail records |
| Repository roots | corpus construction | compatible graph output and source scope | copied snapshot metadata |
| Source text, documents, resolver dictionaries | parsing, resolution, snippets | separate snippet/source owners | not referenced by the compact snapshot |

`internal/navigation/query.go` is the retained reference implementation for
parity. `internal/navigation/filter.go`, `diff.go` and `stats.go` consume complete
compatibility models and are not silently rewritten to calls-only models.
`internal/analysis/graph.go` filters the graph before selecting query roots.
`internal/analysis/architecture.go` consumes callable and type declarations.
CLI graph rendering, architecture/rule consumers and native serialization keep
their existing full-detail contracts. Search's separate related index and
`NavigationAnalysis` still require a follow-up adapter audit; this core does not
claim that those objects have already been replaced.

## Implemented representation

- Snapshot-local `uint32` nodes, call rows and candidate-target offsets/counts.
- Forward and reverse CSR adjacency. Querying a prepared snapshot touches the
  frontier's edges, not every call at every breadth-first level.
- Sparse row indexes for declarations, fields, types, imports, exports, type
  usages and member accesses. Selected output is hydrated in original source
  order without scanning each global fact collection.
- Rich facts are stored in addressable **16-row native protobuf blocks**, reusing
  the existing schema/checksum codec rather than inventing a binary format.
  Blocks larger than 4 KiB are additionally compressed when that saves bytes;
  one encoder scratch buffer is reused during freeze, not kept in the snapshot.
  Compressed length/checksum bounds are verified before native decoding.
  A hydration batch decodes each selected block once. No decoded graph, parser
  document, resolver index or source-content map is held by `querySnapshot`.
- String keys are copied to avoid retaining complete input artifact buffers
  through substrings. Candidate order, direct-target preference, duplicate call
  identities and duplicate declarations follow existing query behavior.
  Caller/direct/candidate identities already in numeric tables are reconstructed
  at hydration rather than duplicated as giant string lists in cold call records.
  Candidates accompanying a direct target remain in details and are preserved.
- Integer capacity is checked before numeric conversions. Corrupt, incomplete
  or wrong-identity detail blocks fail explicitly instead of returning a valid
  looking partial graph.
- Type rows are generation-local, source-ordered identities: identical names,
  modules and paths are never coalesced into a terminal-name type identity.
  These IDs are not placed in persisted cursors or external links.
- Existing reusable `analysis` now keeps its compatibility graph rather than
  retaining the complete resolver `navigationIndex` and its source maps.

Caller-owned opt-in:

```go
snapshot, err := navigation.NewGraphOperations().PrepareQuery(resolvedGraph)
if err != nil { return err }
selected, err := snapshot.Query(rootIDs, navigation.NavigationQueryCallees, 2)
```

`GraphEngine.BuildQueryFiles` also builds and freezes a source corpus once,
returning the snapshot, original source statistics and an explicit error.
`QuerySnapshot.Graph()` is an explicit full-output hydration; it is not a cache
of a decoded graph. Both interfaces are reached through existing service
constructors, not a parallel navigation engine.

For development comparisons only, `GREPPLE_EXPERIMENTAL_COMPACT_NAVIGATION=1`
switches existing graph queries to the new implementation. This one-shot path
includes preparation cost. Reusing a prepared snapshot is required to obtain the
bounded-query measurements below. The switch is not enabled by default and is
not a remote request option. Mutable source freshness and existing revision
fences are unchanged.

## Correctness evidence

Differential tests compare the complete ordered graph for callers, callees,
dependencies, dependents and impact, depths 1–4, single/multiple roots,
root-exclusion rules, cycles, candidates, unknown/external endpoints, duplicate
identities, every rich fact category, and deterministic generated graphs.
Actual parser/resolver fixtures cover all 18 currently navigation-capable
adapters; adding an adapter without a fixture fails the test. Concurrent-reader
race tests, input-mutation detachment and corrupted-detail tests are included.
Focused search/analysis/CLI graph race tests also pass with the development
query switch enabled. Opt-in differential queries on the preserved 111.9 MiB
artifact compare all five directions and three source-ordered roots.

## Measured baseline and current prototype

These are isolated test/artifact measurements, **not deployed Codex request
memory claims**. Same host/corpus, `GOMAXPROCS=2`, `GOMEMLIMIT=384MiB`; retained
heap is sampled after a forced test-only GC. Production code contains no forced
GC or cache flush. Logs/profiles are under `/workspace/.tmp/nav-snapshot-measure`.

| Measurement | Reference | Current compact prototype |
| --- | --- | --- |
| Synthetic 10,000 declarations / 100,000 rich calls: retained heap | 53,029,832 B | 18,822,240 B (**64.5% less**) |
| Same synthetic process: maximum RSS | 94,136 KiB | 165,020 KiB (**75% higher; rollout gate**) |
| Synthetic freeze conversion | not applicable | ~187 ms, separate from parsing/building |
| Preserved artifact: 55,370 declarations / 591,422 calls: retained heap | 440,804,288 B | 221,482,840 B (**49.8% less**) |
| Preserved artifact: maximum RSS (decode + freeze, not parsing) | 732,860 KiB | 812,180 KiB (**10.8% higher**) |
| Artifact full decode / compact conversion | ~1.98 s decode | ~4.59 s freeze **in addition to decode** |
| Reused depth-2 neighborhood, 1,000 declarations/calls | ~265–276 µs; 409,280 B/query; 3,016 allocations | ~17 µs; 33,152 B/query; 137 allocations |
| Same neighborhood, 10,000 declarations/calls | ~2.31–2.43 ms; 3,959,536 B/query; 30,044 allocations | ~16 µs; 33,152 B/query; 137 allocations |
| 200 reused synthetic 10,000-node queries, p50/p95 | 2.379 / 3.997 ms | 11.582 / 20.663 µs |

The prepared traversal/hydration cost stays approximately constant as unrelated
corpus size grows. Heap profiles retain native byte blocks, compact arrays and
lookup metadata, not the unused builder/source maps. These figures do **not**
include production source-tree parsing, disk cold-start behavior or the cost of
preparing a fresh snapshot on every request. Full-output `Graph()` intentionally
materializes full facts; bounded queries do not.

The real artifact disproved the first synthetic-only prototype: encoding full
candidate strings into every 16-row detail block raised retained heap to
549,230,448 B. Compression alone still used 404,652,072 B. Moving caller/target
identities to hot numeric tables and reconstructing their exact strings during
hydration produced the current 221,482,840 B result. No candidate relationships
were discarded to achieve this reduction. The original 128-row experiment also
allocated ~242 KiB per small query; 16-row blocks cut that substantially.

Peak RSS is still worse, particularly on the synthetic fixture. The opt-in
prototype is **not approved for default production use** just because retained
heap and repeated-query latency improve. The disk-backed/query-ready generation
work must avoid full decode plus snapshot coexistence and repeat the same tests.
Cold-disk query p95, actual persisted disk bytes, reload without full decode and
separate production build/query RSS remain unmeasured.

Reproduce:

```sh
GOMAXPROCS=2 GOMEMLIMIT=384MiB go test -race -p 1 ./internal/navigation
GOMAXPROCS=2 GOMEMLIMIT=384MiB go test -p 1 ./internal/navigation \
  -run '^$' -bench BenchmarkSnapshotNeighborhood -benchmem -count 3
GREPPLE_SNAPSHOT_MEASURE=compact GREPPLE_SNAPSHOT_PROFILE=/private/path/compact.heap \
  GOMAXPROCS=2 GOMEMLIMIT=384MiB go test -p 1 ./internal/navigation \
  -run '^TestSnapshotHeapMeasurement$' -v
```

## Remaining rollout gates

1. Bound freeze peak allocation; do not enable the development switch by default
   while conversion substantially raises the measured synthetic process peak.
2. Move native detail blocks behind an addressable disk reader with explicit
   ownership and generation pinning; measure cold reads, disk bytes, reload,
   query p50/p95 and separate build/query RSS on identical real corpora.
3. Integrate prepared snapshots into backend immutable artifact use and search
   related projection, preserving its full type/import/source evidence and
   revision fences. Avoid a new full source graph for every selected search page.
4. Build all outdated navigation artifacts proactively, independently of request
   enrichment; fix the separately observed Codex dependency-manifest failure.
5. Add persistent query-ready sections/atomic generations only after the current
   representation passes memory gates. No automatic historical artifact cleanup
   is introduced here. Incremental dependency-directed re-resolution remains a
   later phase, not a correctness shortcut.
