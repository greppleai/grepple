# Architecture performance benchmarks

`BenchmarkArchitectureWorkflows` measures the four architecture retrieval paths that agents and schema tooling use most often:

```bash
make architecture-benchmark
```

The deterministic fixture contains a three-file TypeScript focused graph and a Go module with eight model files plus one dependent package. Every operation validates successful generation and reports standard time/allocation metrics plus `output_bytes/op`.

## Reviewed baseline and budgets

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=10x`:

| Workflow | Baseline time | Baseline allocated bytes | Output bytes | Review budget time | Review budget allocated bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Focused structure | 1.59 ms | 0.68 MB | 722 | 3 ms | 1 MB |
| Focused flow | 1.49 ms | 0.46 MB | 520 | 3 ms | 0.75 MB |
| Package bundle | 20.28 ms | 5.37 MB | 14,110 | 35 ms | 8 MB |
| Workspace bundle | 4.54 ms | 1.01 MB | 2,475 | 10 ms | 2 MB |

Budgets are review thresholds rather than flaky test assertions: benchmark time depends on host scheduling and filesystem behavior. A repeatable result above either budget requires profiling or an explicit baseline review before merge. Correctness remains test-gated, while output size is deterministic and should not grow without explaining the added agent value.

Run the benchmark on the same machine and Go version when comparing changes. Use at least three samples for a budget decision; treat one timing outlier as diagnostic rather than conclusive.

## Per-file navigation fact cache

`BenchmarkNavigationFactCache` measures a 101-callable Go source as a forced cold parse/write and as a warm content-and-grammar-addressed read:

```bash
go test ./parser -run '^$' -bench '^BenchmarkNavigationFactCache$' -benchtime=10x -benchmem
```

On the baseline host, the initial reviewed sample measured 8.80 ms cold and 0.47 ms warm. The benchmark also asserts the expected miss/hit state. Output parity is enforced separately by parser and CLI tests so performance cannot justify cache-dependent results.
