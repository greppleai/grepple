# Architecture performance benchmarks

`BenchmarkArchitectureWorkflows` measures focused Mermaid structure/flow generation, while `BenchmarkDirectoryArchitecture` measures language-neutral directory indexing and JSON projection:

```bash
make architecture-benchmark
```

The deterministic fixtures contain a three-file TypeScript focused graph and a 20-directory Go repository. Every operation validates successful generation and reports standard time/allocation metrics plus `output_bytes/op`.

## Reviewed baseline and budgets

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=10x`:

| Workflow | Baseline time | Baseline allocated bytes | Output bytes | Review budget time | Review budget allocated bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| Focused structure | 1.89 ms | 0.95 MB | 722 | 3 ms | 1 MB |
| Focused flow | 1.71 ms | 0.70 MB | 520 | 3 ms | 0.75 MB |
| Directory architecture | 5.63 ms | 0.63 MB | 17,621 | 15 ms | 1 MB |

Budgets are review thresholds rather than flaky test assertions: benchmark time depends on host scheduling and filesystem behavior. A repeatable result above a budget requires profiling or an explicit baseline review before merge. Directory JSON grew by 2,770 bytes from the pre-classification baseline to expose source classifications and explicit import/type relation coverage rather than hiding uncertainty. Correctness remains test-gated, while output size is deterministic and should not grow without explaining the added agent value.

Run the benchmark on the same machine and Go version when comparing changes. Use at least three samples for a budget decision; treat one timing outlier as diagnostic rather than conclusive.

## Per-file navigation fact cache

`BenchmarkNavigationFactCache` measures a 101-callable Go source as a forced cold parse/write and as a warm content-and-grammar-addressed read:

```bash
go test ./parser -run '^$' -bench '^BenchmarkNavigationFactCache$' -benchtime=10x -benchmem
```

On the baseline host, the initial reviewed sample measured 8.80 ms cold and 0.47 ms warm. The benchmark also asserts the expected miss/hit state. Output parity is enforced separately by parser and CLI tests so performance cannot justify cache-dependent results.
