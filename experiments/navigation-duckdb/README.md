# Navigation storage experiment: DuckDB versus Go

This is an **isolated nested Go module**, not a production storage migration.
The parent module, CLI, backend, navigation behavior, and credentials are unchanged.

See [COLUMNAR.md](COLUMNAR.md) for the separately measured typed-column variant,
which improves space and broad-query tails but not overall median latency.

## Results on this machine

Measured 2026-10-04 with DuckDB 1.5.5 / duckdb-go v2.10505.0,
Go 1.27.0, Linux amd64, Intel Core Ultra 7 165H, and approximately 3.8 GiB RAM.
DuckDB uses one thread and a 512 MB memory limit; Go uses GOMAXPROCS=4.
Full build identities and raw measurements are in [`results/`](results/).

The source corpus is the tracked, non-test, non-generated Go code from the local
Grepple and backend checkouts: **598 files, 5,310 declarations, 24,388 calls**, all
parsed without failures/recovery. The 10× dataset is **53,100 declarations and
243,880 calls**, produced by replicating that resolved graph into disconnected
namespaces. It is a scaling fixture, **not ten independently indexed repositories**.
Type-owner names are isolated between replicas to prevent accidental context growth.

### Fresh CLI process: complete immutable snapshot query

Median wall time includes process startup, loading/opening storage, graph querying,
result JSON encoding/digest, and shutdown. The filesystem page cache is warm.

| Dataset | Current query + production protobuf | Go adjacency + protobuf | DuckDB |
| --- | ---: | ---: | ---: |
| Actual corpus | 68.9 ms | 73.1 ms | 114.1 ms |
| 10× graph | 471.9 ms | 499.6 ms | 138.4 ms |

DuckDB is **1.65× slower** on the actual corpus, but **3.41× faster** at 10× scale
(71% less wall time), mainly because it avoids decoding the entire graph.

### Long-lived local daemon: fresh client process per request

Each prototype daemon loads its backend once, serves complete result graphs over a
private Unix socket, and recomputes queries rather than caching exact reports.
Median wall time includes client startup, IPC, response decoding, and shutdown.

| Dataset | Resident current Go query | Resident Go adjacency | Resident DuckDB |
| --- | ---: | ---: | ---: |
| Actual corpus | 14.6 ms | 7.9 ms | 33.1 ms |
| 10× graph | 65.0 ms | 7.0 ms | 27.6 ms |

DuckDB is **2.27× slower** on the actual corpus and **2.36× faster** at 10× scale.
However, 10× daemon p95 is **145.0 ms with DuckDB versus 126.8 ms with current Go**;
it does not improve every query or tail latency. Go adjacency is about **3.94×
faster than DuckDB** on the larger daemon workload.

### Resident query core, excluding process/IPC and result encoding

| Dataset | Current Go p50 / p95 | Go adjacency p50 / p95 | DuckDB p50 / p95 |
| --- | ---: | ---: | ---: |
| Actual corpus | 5.02 / 9.90 ms | 0.18 / 9.08 ms | 9.18 / 98.90 ms |
| 10× graph | 54.39 / 79.36 ms | 0.21 / 7.88 ms | 9.08 / 109.78 ms |

### Space, memory, and build cost

| Dataset | Protobuf file | DuckDB file | Current Go peak RSS | Go adjacency peak RSS | DuckDB peak RSS |
| --- | ---: | ---: | ---: | ---: | ---: |
| Actual corpus | 5.6 MiB | 23.8 MiB | 82.8 MiB | 82.9 MiB | 166.1 MiB |
| 10× graph | 52.0 MiB | 206.5 MiB | 526.4 MiB | 537.3 MiB | 191.8 MiB |

RSS is process high-water memory including load and measured workload, **not steady
live heap**. It includes native DuckDB allocations. DuckDB is approximately four
times larger on disk; JSON payload columns favor exact semantic reuse over an
aggressively normalized database design. DuckDB import/index/checkpoint takes
approximately **0.82 s / 5.17 s** after source resolution. Source parsing/resolution
itself takes approximately 9–11 seconds and is not accelerated by this prototype.
The linked prototype binaries are approximately 39 MiB (Go) and 105 MiB (DuckDB).

## Methodology and correctness

- Baseline invokes the real `navigation.NewGraphOperations().Query`.
- Protobuf baseline uses the exact production `MarshalNavigationFactArtifact` /
  `UnmarshalNavigationFactArtifact` codecs, including checksum verification.
- DuckDB stores declaration/call rows, target edges, and all ancillary facts, with
  ART indexes, bulk appenders, read-only opens, and cached prepared statements.
  Small predicates use scalar bindings; larger frontiers use vector lists.
- SQL and Go adjacency preselect relevant edges/facts; production's own bounded
  projection preserves order, candidates, cycles, context facts and owned facts.
- All **12 mixed workload queries** match the baseline's entire result JSON digest
  in every timed CLI/daemon/resident request, not merely row counts.
- Separate tests verify all five traversal directions, depths 1–10, multiple roots,
  ambiguous/unresolved/unknown targets, cyclic graphs, context facts and errors.
- Resident measurements: one warm-up of all cases, then 7 mixed rounds (84 samples).
  CLI and daemon: 2 mixed rounds each (24 fresh client/process samples per backend).
- A JSON loading control initially exaggerated the benefit. It is deliberately
  **not** the primary baseline because production already uses compact protobuf.

## Important limits

These are **immutable-snapshot storage/query measurements**, not end-to-end
production replacement benchmarks. Discovery, source-byte fingerprinting,
revision admission, dependency re-resolution, live updates, and cache invalidation
are excluded for every prototype backend. DuckDB has not been wired into `greppled`.
The real daemon caches exact selector reports and revalidates source snapshots;
the prototype isolates resident query/storage cost and does not replace that policy.
A separate sanity test on `internal/navigation` measured the stock CLI at ~358 ms
and the stock exact-report-cache daemon path at ~169 ms, with identical output.
Those figures use a different scope and **must not** be divided by prototype times
as an alleged production DuckDB speedup.

Only Linux, a single reader, warm filesystem pages, one DuckDB thread, and the
listed data shapes were tested. No cold-device, incremental update, concurrent
reader/writer, crash recovery, startup invalidation, or production packaging claims.
The SQL schema and statement count can be optimized further; this is not a verdict
on every possible DuckDB design. Background services remain running during tests.

## Recommendation

For this point-traversal workload, first explore **persistent Go adjacency indexes
in the daemon plus the existing protobuf snapshots**: it captures the algorithmic
benefit without DuckDB query-planning/CGO overhead or a larger runtime.
DuckDB merits further work if avoiding full CLI graph materialization at much larger
scale, bounded memory, SQL analytics, or incremental relational storage is a goal.
Do not migrate production based solely on these results.

## Reproduce

```sh
cd experiments/navigation-duckdb
go test -tags duckdb ./...
go test -race -tags duckdb ./...
# Creates a private data directory; refuses to overwrite an existing database.
REPOS=/workspace/grepple,/workspace/grepple-backend bash prepare.sh /workspace/.tmp/nav-duckdb-new
```

`prepare.sh` builds separate Go-only and DuckDB-linked binaries and invokes
`run.py`. Artifacts and sockets remain outside the repository. Module compilation
uses `-mod=mod` because an inherited Swift/Dart dependency-test import problem
prevents `go mod tidy`; ordinary builds/tests work. No production module is changed.