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
| `BroadAccidental` | How much output does an accidentally broad body search retrieve? | 1 |
| `OutlineDiscovery` | What declarations does this file contain? | 1 |
| `StructuralLookup` | Locate and retrieve one implementation. | 1 |
| `LineLocateThenAt` | Locate a line with its construct extent, then retrieve that declaration. | 2 |
| `RelatedNavigation` | What immediately calls this and what does it call? | 1 |
| `ImpactGraph` | What is the bounded bidirectional impact neighborhood? | 1 |
| `ArchitectureResolve` | Which directory owns this declaration and what is its exact range? | 1 |
| `EnclosingScope` | What nearest syntax scope owns this body-line match? | 1 |
| `EditLocation` | Where is the exact edit-ready evidence line? | 1 |

The benchmark reports:

- `tool_calls/op`: modeled CLI retrieval calls required by the workflow;
- `retrieved_bytes/op`: bytes returned to the agent;
- `approx_tokens/op`: returned bytes divided by four, used only as a stable rough comparison;
- standard `ns/op`, allocation bytes, and allocations from Go's benchmark runner.

A benchmark fails rather than reporting metrics when required answer fragments are missing. Workflows that can truncate must also retain an explicit truncation or omission marker in their correctness expectations when the fixture reaches the relevant bound.

## Output-budget evaluation

A synthetic 800-match body search measured the broad-query failure mode directly. At the previous 40960-byte default it could consume roughly 10K approximate tokens before guidance appeared. The 16384-byte human default returns about 16.3 KB/~4.1K approximate tokens including explicit truncation guidance. Complete output above the repository spill threshold now returns a small artifact descriptor instead of injecting the full document into agent context; `--no-spill` is the explicit original-stream escape hatch.

## Initial baseline

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=10x`:

| Workflow | Calls | Retrieved bytes | Approx. tokens | Time/op |
| --- | ---: | ---: | ---: | ---: |
| `BreadthSummary` | 1 | 18 | 4.5 | 4.87 ms |
| `OutlineDiscovery` | 1 | 77 | 19.25 | 2.72 ms |
| `StructuralLookup` | 1 | 166 | 41.5 | 2.64 ms |
| `LineLocateThenAt` | 2 | 152 | 38 | 5.07 ms |
| `RelatedNavigation` | 1 | 307 | 76.75 | 8.10 ms |
| `ImpactGraph` | 1 | 978 | 244.5 | 6.33 ms |
| `ArchitectureResolve` | 1 | 173 | 43.25 | 5.75 ms |
| `EnclosingScope` | 1 | 53 | 13.25 | 2.87 ms |
| `EditLocation` | 1 | 40 | 10 | 2.69 ms |

Direct `ArchitectureResolve` replaces the former package-summary fixture for known declarations. It returns the owner and exact range in 173 bytes instead of the former 337-byte package summary, a 49% reduction, while remaining one retrieval call. The structural lookup similarly returns slightly more text than line-only plus `--at`, but removes one retrieval round trip. Timing is machine-dependent; call and fixture-output metrics are the primary regression signals until statistically reviewed budgets are established.

## Navigation-resolution measurement extension

Compact graph headers now report total calls, visible local edges, and resolved, ambiguous, and unresolved counts. The fixed `ImpactGraph` workflow grew from 926 bytes/~231.5 tokens to 978 bytes/~244.5 tokens—52 bytes/~13 tokens—to expose whether a focused impact result depends materially on ambiguous syntax resolution. Complete JSON additionally reports singleton candidates, confidence counts, and ambiguity frequency overall and by language family. Other fixed workflow output sizes are unchanged.

## Construct-range extension

After `--line-only` gained parser-backed `PATH:START-END` locations, the fixed function lookup returned 152 bytes/~38 tokens instead of 150 bytes/~37.5 tokens. The two added bytes disclose the exact `2-6` function extent for the next Read or `--at` call. A match inside a construct that does not begin one remains unchanged by default; `EditLocation` therefore still returns 40 bytes/~10 tokens.

Opt-in `--enclosing` adds the nearest syntax scope while retaining the actual match line. The fixed `EnclosingScope` workflow returns `service.go:5@2-6` in one 53-byte/~13.25-token call, allowing the next Read to target the owner without an intermediate outline or `--at` discovery call. Across repeated 10-iteration samples, enclosing-scope lookup took 1.2–1.4 ms/op, ordinary ranged lookup took 2.2–2.9 ms/op, and exact edit location took 1.2–1.5 ms/op. Timing remains secondary to the stable call/output metrics.
