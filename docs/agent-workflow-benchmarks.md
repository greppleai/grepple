# Agent workflow benchmarks

Grepple measures end-to-end retrieval workflows in addition to parser and matcher microbenchmarks. The goal is to detect regressions in the quantities that affect an agent directly: answer correctness, tool-call count, returned bytes, approximate tokens, elapsed time, and disclosed truncation.

## Run

```bash
make agent-benchmark
```

For before/after comparisons, collect multiple samples and use `benchstat`:

```bash
go test ./internal/cli -run '^$' -bench '^BenchmarkAgentWorkflows$' -count 10 > before.txt
# apply the change
go test ./internal/cli -run '^$' -bench '^BenchmarkAgentWorkflows$' -count 10 > after.txt
benchstat before.txt after.txt
```

## Fixed workflows

`BenchmarkAgentWorkflows` creates a deterministic two-file Go fixture and verifies required answer fragments on every iteration.

| Workflow | Question represented | CLI retrieval calls |
| --- | --- | ---: |
| `BreadthSummary` | How broad is this term before loading bodies? | 1 |
| `OutlineDiscovery` | What declarations does this file contain? | 1 |
| `StructuralLookup` | Locate and retrieve one implementation. | 1 |
| `LineLocateThenAt` | Locate a line with its construct extent, then retrieve that declaration. | 2 |
| `RelatedNavigation` | What immediately calls this and what does it call? | 1 |
| `ImpactGraph` | What is the bounded bidirectional impact neighborhood? | 1 |
| `ArchitectureSummary` | What is the bounded package ownership and public surface? | 1 |
| `EditLocation` | Where is the exact edit-ready evidence line? | 1 |

The benchmark reports:

- `tool_calls/op`: modeled CLI retrieval calls required by the workflow;
- `retrieved_bytes/op`: bytes returned to the agent;
- `approx_tokens/op`: returned bytes divided by four, used only as a stable rough comparison;
- standard `ns/op`, allocation bytes, and allocations from Go's benchmark runner.

A benchmark fails rather than reporting metrics when required answer fragments are missing. Workflows that can truncate must also retain an explicit truncation or omission marker in their correctness expectations when the fixture reaches the relevant bound.

## Initial baseline

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=10x`:

| Workflow | Calls | Retrieved bytes | Approx. tokens | Time/op |
| --- | ---: | ---: | ---: | ---: |
| `BreadthSummary` | 1 | 18 | 4.5 | 1.76 ms |
| `OutlineDiscovery` | 1 | 77 | 19.25 | 1.34 ms |
| `StructuralLookup` | 1 | 166 | 41.5 | 1.36 ms |
| `LineLocateThenAt` | 2 | 150 | 37.5 | 2.35 ms |
| `RelatedNavigation` | 1 | 307 | 76.75 | 3.09 ms |
| `ImpactGraph` | 1 | 756 | 189 | 3.10 ms |
| `ArchitectureSummary` | 1 | 337 | 84.25 | 2.24 ms |
| `EditLocation` | 1 | 40 | 10 | 1.09 ms |

The structural lookup illustrates the intended tradeoff: it returns slightly more text than line-only plus `--at`, but removes one retrieval round trip. Timing is machine-dependent; call and fixture-output metrics are the primary regression signals until statistically reviewed budgets are established.

## Construct-range extension

After `--line-only` gained parser-backed `PATH:START-END` locations, the fixed function lookup returned 152 bytes/~38 tokens instead of 150 bytes/~37.5 tokens. The two added bytes disclose the exact `2-6` function extent for the next Read or `--at` call. A match inside a construct that does not begin one remains unchanged; `EditLocation` therefore still returns 40 bytes/~10 tokens. Across repeated 10-iteration samples, the ranged workflow took 2.4–2.7 ms/op and exact edit location took 1.3–1.5 ms/op. Timing remains secondary to the stable call/output metrics.
