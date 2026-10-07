# DuckDB-oriented columnar variant

The `duckdb-col` variant preserves the same immutable-snapshot graph questions:
callers, callees, dependencies, dependents and bidirectional impact, including
multiple roots, candidates, unresolved targets, cycles, original ordering and
all context facts. Production code and production dependencies remain unchanged.

## Storage and query changes

- **Every fact field is a native DuckDB column**: VARCHAR, BIGINT, BOOLEAN or
  VARCHAR[]. No stored JSON/protobuf payload or duplicated row-serialization cache.
- Tables retain `_ord` for stable ordering and `_key` for ART lookup. Calls have an
  ordinal index; the separate target-edge relation indexes incoming calls.
- Traversal fetches only call ordinal, caller and target information. Complete call
  metadata is loaded once during final projection rather than decoded at each level.
- Exact persisted collection counts allow empty tables to be skipped. Empty key
  selections also skip SQL, without treating approximate statistics as authoritative.
- Both variants cache prepared point predicates and use list predicates for larger
  frontiers; statement cache access is synchronized for concurrent readers.
- Existing Go projection remains the semantic oracle. This is **not** a complete
  recursive-SQL engine or a replacement for production snapshot validation.
- Removed the prototype-only depth-eight restriction; positive depths supported by
  the graph operation remain supported, with depths 1–10 explicitly tested.

## Measurements

[`results/columnar/`](results/columnar/) contains a fresh comparison of all four
backends against exactly the same previously exported graphs. Filesystem caches
are warm; the daemon keeps its database open. The methodology and limits from
[README.md](README.md) still apply. This run is separate from the original results,
and background load/timing variability means comparisons should use one run's
controls rather than mix numbers across runs.

| Dataset | JSON-row DuckDB query p50 / p95 | Columnar DuckDB query p50 / p95 |
| --- | ---: | ---: |
| Actual corpus | 11.17 / 114.11 ms | 15.45 / 85.62 ms |
| 10× fixture | 11.91 / 130.09 ms | 15.46 / 91.50 ms |

Query timings exclude process startup, IPC and final result encoding.

| Dataset | DuckDB file: original → columnar | CLI wall p50: original → columnar | Daemon wall p50: original → columnar |
| --- | ---: | ---: | ---: |
| Actual corpus | 23.8 → 16.8 MiB | 120.8 → 151.7 ms | 34.8 → 48.3 ms |
| 10× fixture | 206.5 → 114.8 MiB | 145.3 → 167.5 ms | 32.0 → 37.6 ms |

At 10×, storage shrank **44%** and query p95 improved **30%**. However, query p50
worsened **30%**, CLI wall p50 worsened **15%**, and daemon wall p50 worsened **18%**.
Peak resident RSS did not improve (183.4 → 193.8 MiB). This is a storage and broad-
query-tail improvement, **not an overall latency win**. The Go adjacency control
still has the lowest daemon p50 in this run (8.3 ms at 10×).

## What was tried, and what remains

Intermediate native STRUCT rows, single-row nested aggregate retrieval, and SQL-
generated JSON transport were tried before choosing direct flat-column retrieval.
They were not better overall. Flat column scans still incur many driver value
conversions, multiple SQL executions and query planning; the measurements do not
prove which cost dominates. Arrow/chunk-based retrieval, fewer SQL round trips,
and integer-key recursive traversal are useful next experiments, but are **not**
implemented or claimed here. No query-result cache was introduced to manufacture
warm-request improvements.

## Correctness and reproduction

- All 12 real-corpus mixed workload results match the current native query's full
  JSON digest in every timed resident, CLI and daemon request, for both graph sizes.
- All five directions, multiple roots and depths 1–10 pass fixture parity tests.
- Every field of all eight fact types round-trips, including Unicode, quotes,
  newlines, named string types, booleans, integer ranges and NULL versus empty lists.
- Concurrent query tests pass under Go's race detector. This is not a concurrency
  throughput benchmark; DuckDB still uses one connection and one thread.

```sh
cd experiments/navigation-duckdb
go test -race -tags duckdb ./...
# New private directory; generates both original and columnar databases.
REPOS=/workspace/grepple,/workspace/grepple-backend bash prepare.sh /workspace/.tmp/nav-columnar-new
# Rerun comparisons on those immutable snapshots:
python3 run.py --data /workspace/.tmp/nav-columnar-new
```

Keep both variants available. Do not switch production storage based on this
experiment: the columnar trade-offs do not yet justify replacing either the
existing protobuf cache or the faster resident Go adjacency approach.