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

`BenchmarkAgentWorkflows` creates a deterministic multi-directory Go fixture and verifies required answer fragments on every iteration.

| Workflow | Question represented | CLI retrieval calls |
| --- | --- | ---: |
| `BreadthSummary` | How broad is this term before loading bodies? | 1 |
| `BroadAccidental` | How much output does an accidentally broad body search retrieve? | 1 |
| `OutlineDiscovery` | What declarations does this file contain? | 1 |
| `StructuralLookup` | Locate and retrieve one implementation. | 1 |
| `LineLocateThenAt` | Locate a line with its construct extent, then retrieve that declaration. | 2 |
| `RelatedNavigation` | What immediately calls this and what does it call? | 1 |
| `ImpactGraph` | What is the bounded bidirectional impact neighborhood? | 1 |
| `DirectoryOrientation` | What are the bounded physical owners and typed relation kinds? | 1 |
| `ArchitectureResolve` | Which directory owns this declaration and what is its exact range? | 1 |
| `RelationExplanation` | What exact call/import/type evidence connects two directories? | 1 |
| `EnclosingScope` | What nearest syntax scope owns this body-line match? | 1 |
| `EditLocation` | Where is the exact edit-ready evidence line? | 1 |
| `ArtifactDirectComplete` | How much context does a complete 1 MiB result consume with spill disabled? | 1 |
| `ArtifactFallback` | Can one bounded artifact read recover the required answer after spill? | 2 |

The benchmark reports:

- `tool_calls/op` or `retrieval_turns/op`: modeled retrievals required by the workflow;
- `retrieved_bytes/op` or `context_bytes/op`: bytes returned to the agent;
- `artifact_reads/op`: bounded artifact reads after descriptor delivery;
- `approx_tokens/op`: returned context bytes divided by four, used only as a stable rough comparison;
- standard `ns/op`, allocation bytes, and allocations from Go's benchmark runner.

A benchmark fails rather than reporting metrics when required answer fragments are missing. Workflows that can truncate must also retain an explicit truncation or omission marker in their correctness expectations when the fixture reaches the relevant bound.

## Output-budget evaluation

A synthetic 800-match body search measured the broad-query failure mode directly. At the previous 40960-byte default it could consume roughly 10K approximate tokens before guidance appeared. The 16384-byte human default returns about 16.3 KB/~4.1K approximate tokens including explicit truncation guidance. The answer-gated artifact fixture compares a complete 1 MiB JSON stream with descriptor delivery plus one 512-byte tail read. `--no-spill` used 1,048,676 context bytes/~262K tokens in one turn. Artifact fallback recovered the same required answer using 902 context bytes/~225.5 tokens, one artifact read, and two retrieval turns—a 99.91% context reduction.

## Initial baseline

Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=10x`:

| Workflow | Calls | Retrieved bytes | Approx. tokens | Time/op |
| --- | ---: | ---: | ---: | ---: |
| `BreadthSummary` | 1 | 18 | 4.5 | 4.87 ms |
| `OutlineDiscovery` | 1 | 77 | 19.25 | 2.72 ms |
| `StructuralLookup` | 1 | 166 | 41.5 | 2.64 ms |
| `LineLocateThenAt` | 2 | 152 | 38 | 5.07 ms |
| `RelatedNavigation` | 1 | 307 | 76.75 | 8.10 ms |
| `ImpactGraph` | 1 | 1,081 | 270.25 | 8.02 ms |
| `DirectoryOrientation` | 1 | 903 | 225.75 | 6.63 ms |
| `ArchitectureResolve` | 1 | 206 | 51.5 | 5.82 ms |
| `RelationExplanation` | 1 | 366 | 91.5 | 5.89 ms |
| `EnclosingScope` | 1 | 53 | 13.25 | 2.87 ms |
| `EditLocation` | 1 | 40 | 10 | 2.69 ms |
| `ArtifactDirectComplete` | 1 | 1,048,676 | 262,169 | 5.58 ms |
| `ArtifactFallback` | 2 | 902 | 225.5 | 10.50 ms |

The three architecture tasks are independently answerable in one retrieval call: bounded orientation uses 903 bytes, direct ownership resolution 206 bytes, and exact relation explanation 366 bytes. Running all three would retrieve 1,475 bytes/~369 tokens, while an agent with a known symbol or directory pair can skip orientation entirely. Direct `ArchitectureResolve` remains 39% smaller than the former 337-byte package summary while adding source classification and entrypoint status. The structural lookup similarly returns slightly more text than line-only plus `--at`, but removes one retrieval round trip. Timing is machine-dependent; call and fixture-output metrics are the primary regression signals until statistically reviewed budgets are established.

## Navigation-resolution measurement extension

Compact graph headers now report total calls, visible local edges, resolution outcomes, entrypoints, and routes; declarations expose entrypoint status. The fixed `ImpactGraph` workflow is 1,081 bytes/~270 tokens, retaining bounded resolution and architecture-role context. Complete JSON additionally reports singleton candidates, confidence counts, ambiguity frequency, and adapter-evidenced route facts.

## Construct-range extension

After `--line-only` gained parser-backed `PATH:START-END` locations, the fixed function lookup returned 152 bytes/~38 tokens instead of 150 bytes/~37.5 tokens. The two added bytes disclose the exact `2-6` function extent for the next Read or `--at` call. A match inside a construct that does not begin one remains unchanged by default; `EditLocation` therefore still returns 40 bytes/~10 tokens.

Opt-in `--enclosing` adds the nearest syntax scope while retaining the actual match line. The fixed `EnclosingScope` workflow returns `service.go:5@2-6` in one 53-byte/~13.25-token call, allowing the next Read to target the owner without an intermediate outline or `--at` discovery call. Across repeated 10-iteration samples, enclosing-scope lookup took 1.2–1.4 ms/op, ordinary ranged lookup took 2.2–2.9 ms/op, and exact edit location took 1.2–1.5 ms/op. Timing remains secondary to the stable call/output metrics.
