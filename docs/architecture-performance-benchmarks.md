# Architecture performance benchmarks

`BenchmarkArchitectureWorkflows` measures focused Mermaid structure/flow generation, while `BenchmarkDirectoryArchitecture` measures language-neutral directory indexing and JSON projection:

```bash
make architecture-benchmark
```

The deterministic fixtures contain a three-file TypeScript focused graph and a 20-directory Go repository. Every operation validates successful generation and reports standard time/allocation metrics plus `output_bytes/op`.

## Reviewed baseline and budgets

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=20x`:

| Workflow | Baseline time | Baseline allocated bytes | Output bytes | Review budget time | Review budget allocated bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Focused structure | 3.03 ms | 0.97 MB | 722 | 3.5 ms | 1 MB |
| Focused flow | 2.37 ms | 0.71 MB | 520 | 3 ms | 0.75 MB |
| Directory architecture | 5.31 ms | 0.68 MB | 20,639 | 15 ms | 1 MB |

Budgets are review thresholds rather than flaky test assertions: benchmark time depends on host scheduling and filesystem behavior. A repeatable result above a budget requires profiling or an explicit baseline review before merge. These baselines use the median of three 20-iteration samples. Directory JSON includes the selected source-file inventory needed for exact determinism diagnostics alongside adapter-evidenced process entrypoints, source classifications, repository roots, and import/type relation coverage. Directory analysis derives outlines and graph facts from one parsed document per selected source. Correctness remains test-gated, while output size is deterministic for the same source paths and should not grow without explaining the added agent value.

Run the benchmark on the same machine and Go version when comparing changes. Use at least three samples for a budget decision; treat one timing outlier as diagnostic rather than conclusive.

## Per-file navigation fact cache

`BenchmarkNavigationFactCache` measures a 101-callable Go source as a forced cold parse/write and as a warm content-and-grammar-addressed read:

```bash
go test ./parser -run '^$' -bench '^BenchmarkNavigationFactCache$' -benchtime=3s -benchmem -count=3

On the baseline host, the current packed-protobuf implementation measured a median 9.05 ms cold and 0.15 ms warm over three three-second samples. The native `NavigationGraph` is encoded directly through a deterministic string-table and packed-column protobuf codec rather than a JSON mirror; entries include a graph checksum and preserve nil-versus-empty slices so cache state cannot change output. The benchmark also asserts the expected miss/hit state. Output parity is enforced separately by parser and CLI tests so performance cannot justify cache-dependent results.
