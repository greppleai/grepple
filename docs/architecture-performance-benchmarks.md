# Architecture performance benchmarks

Historical benchmark rows below include CLI modes removed during simplification; retain their measurements for regression context, not as runnable command suggestions.

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

## Real-checkout CLI benchmark harness

The small deterministic fixtures above do not predict whole-repository architecture cost. Use `scripts/benchmark-repositories.py` with explicit existing Git checkouts or approved HTTPS URLs. The default cases are `tree --depth 1 .` and `architecture directory --depth 2 --max-nodes 80 .`; add `--case NAME='COMMAND ARGS'` for focused lookups or JSON output. Each case runs once with an empty isolated navigation cache, then `--runs - 1` times with that cache warm. A command timeout kills its process group. The checkout stays unchanged: caches, captured output, and any shallow clones live under the new `--output` directory, which cannot be inside a selected checkout.

```bash
go build -o /tmp/grepple-bench-bin ./cmd/grepple
PYTHONDONTWRITEBYTECODE=1 python3 scripts/benchmark-repositories.py \
  --binary /tmp/grepple-bench-bin --repo /path/to/checkout \
  --case 'directory=architecture directory --depth 2 --max-nodes 80 .' \
  --case 'resolve=graph resolve --symbol SomeSymbol .' \
  --runs 2 --timeout 90 --output /tmp/grepple-bench-run
```

Alternatively, `--repo-list /path/to/approved-repos.txt --sample 3 --seed 42` selects a repeatable random subset of local paths and/or HTTPS URLs; `--url https://github.com/owner/repo` shallow-clones one repository. All samples are selected from the explicitly provided universe, not arbitrary GitHub search results. `results.json` records Grepple executable SHA-256, Git revisions/status before and after, commands, elapsed seconds, peak RSS in KiB, stdout/stderr byte counts, parsed source accounting when emitted near the start, exit status, and timeouts. `results.csv` summarizes runs and source counts. `--keep-output` retains full stdout/stderr for debugging. A source count of `null` means that output did not expose complete accounting within the first 8 KiB, not that no source was scanned. Nonzero exits/timeouts make the harness exit nonzero, but it writes the results first. Use `--no-spill` (set by the harness) so output bytes measure actual CLI output rather than an artifact descriptor. Avoid `--production-only` for a repository without fresh kind metadata, since its sources will be unknown.

For CPU/memory attribution on one real checkout, an opt-in Go benchmark builds the full architecture without printing JSON. Keep the profile and cache outside the checkout:

```bash
mkdir -p /tmp/grepple-profiles
GREPPLE_BENCH_REPO=/path/to/checkout GREPPLE_NAVIGATION_CACHE_DIR=/tmp/grepple-profiles/cache \
  go test ./internal/cli/architecture -run '^$' \
  -bench '^BenchmarkDirectoryArchitectureCheckout$' -benchtime=1x \
  -cpuprofile=/tmp/grepple-profiles/cpu.pprof -memprofile=/tmp/grepple-profiles/mem.pprof \
  -o /tmp/grepple-profiles/architecture.test
go tool pprof -top -cum /tmp/grepple-profiles/cpu.pprof
```

### Pi checkout investigation (single-run diagnostic, not a cross-repository budget)

On Linux/amd64, Go 1.25.14, checkout `fleetagent/pi` at `a95987891c838741a09b59b1266aee83a8c2250f`, full architecture discovered 1,063 sources, parsed 844, skipped 219, recovered 3, and failed 0. The checkout lacked Grepple directory metadata, so kind filtering would not be representative. The same checkout and isolated navigation cache per case were used for one cold and one warm wall-clock run of each build:

| Command | Before cold / warm | TS/JS relative-import index cold / warm | Shared navigation indexes cold / warm |
| --- | ---: | ---: | ---: |
| `architecture directory --depth 2 --max-nodes 80 .` | 32.71 / 22.43 s | 16.96 / 7.09 s | 13.91 / 3.68 s |
| `architecture resolve --symbol AgentSession .` | 31.21 / 21.93 s | 16.66 / 6.83 s | 14.16 / 3.77 s |

The first one-iteration profile reported 33.68 s and 8.25 GB allocated: each TS/JS relative import/re-export rescanned the candidate file set. The TS-only change indexed module paths once per language and reduced a follow-up profile to 18.32 s and 2.64 GB allocated. A shared per-corpus path index now serves both TS and JS relative imports and config aliases, Go local/replacement imports, C/C++ quoted includes, and Python module-path candidates. Java/Kotlin and C# use a separate per-corpus export-name index; Rust already uses a module index. Each language retains its matching, disambiguation, ordering, and deduplication rules. Complete Pi architecture JSON is byte-for-byte identical in all three builds (10,728,492 bytes, SHA-256 `b92f54254c722072e464f9280daa79cc1c48216a6e5ea4901dd87dd6d937cb86`).

For a Go-heavy comparison, the local `grepple-backend` checkout (84 parsed files) measured 10.55 / 8.54 s cold/warm for `architecture directory` and 9.34 / 8.09 s for `graph build` before shared indexing, versus 1.97 / 0.77 s and 1.82 / 0.62 s respectively afterwards. Go's ordinary imports already have a package index; the remaining hotspot was repeated reads of `go.mod` for local-import candidate matching. Module identity is now cached per source **within one navigation graph build**, and directory lookups use the shared path index. Its complete architecture JSON was byte-for-byte identical (273,074 bytes, SHA-256 `fe029db6a8645e1832cbe217ac47911198735de5523f4d879d41d25e57fa29e0`). One-iteration Go architecture profiles measured 9.52 s / 262 MB allocated before, and 1.95 s / 198 MB afterwards.

Complete `graph build --json .` output also matched byte-for-byte before and after shared indexing on both checkouts: Pi (60,252,988 bytes, SHA-256 `845b0300618f69bd415d42cae18509ce7ab509b21e0e4a490c67466ae943d146`) and the Go backend (4,382,279 bytes, SHA-256 `01af016fda029200ae4667bae1bd2c0eca96b0543e1f8db66baa8c9e48d3f3cb`). Python, C-family, and JVM/C# resolvers additionally have focused indexed-versus-scan tests; these two repositories are not comprehensive performance samples for those language families.

These timings are diagnostics, not latency guarantees: CPU scheduling, cold/warm disk and cache state, source mix, repository size, and fresh metadata affect results. Parsing and TypeScript configuration resolution remain possible bottlenecks on other repositories. Projection limits such as `--depth` and `--max-nodes` do not avoid full source indexing; narrow paths or use `--max-files` only when a partial result is acceptable.

### Persistent resolved-graph cache (no trigram index)

When `GREPPLE_NAVIGATION_CACHE_DIR` is set, document-based navigation analysis stores the fully resolved graph under its `resolved/` subdirectory, alongside the existing per-file fact cache. The CLI sets its usual navigation cache directory by default; library callers can opt in by setting the environment variable, or bypass both caches with `BuildOptions.DisableCache`. Entries are bounded packed-protobuf artifacts with graph checksums, digest checks, and atomic writes. Missing, corrupt, unreadable, or oversized entries fall back to a fresh graph build without changing the output.

The cache key covers the executable and artifact schema, working directory, ordered source paths and source/grammar digests, plus the existence and contents of ancestor `go.mod`, `go.work`, and `tsconfig.json` files that can affect import resolution. A hit skips cross-file graph construction, **not** source discovery, document parsing, outlines, or search-index construction; source failures, recovery, and truncation are still accounted for by the current invocation. Config files that cannot be safely fingerprinted bypass the resolved-graph cache.

One cold and one warm isolated-cache run on the same checkouts as above (seconds; diagnostic only):

| Full command | Shared indexes cold / warm | Resolved cache cold / warm |
| --- | ---: | ---: |
| Pi `architecture directory --depth 2 --max-nodes 80 .` | 13.91 / 3.68 | 14.96 / 2.97 |
| Pi `architecture resolve --symbol AgentSession .` | 14.16 / 3.77 | 14.61 / 2.87 |
| Go backend `architecture directory --depth 2 --max-nodes 80 .` | 1.97 / 0.77 | 1.87 / 0.47 |
| Go backend `graph build .` | 1.82 / 0.62 | 1.87 / 0.37 |

Complete cold and warm architecture and graph JSON remained byte-for-byte identical to the pre-cache output on both checkouts. Cold runs may be slower because they serialize the graph; large repositories retain parsing and source-selection costs even on a cache hit. The benchmark harness above provides isolated caches and records checkout revision, source accounting, elapsed time, and peak RSS for additional repositories.

### One optional foreground user-level report cache

`make bin/grepple bin/greppled` builds both commands. Start one `greppled` from any directory. From a checkout, run `grepple --daemon architecture directory --json .`, `grepple --daemon graph resolve --symbol Symbol .`, or `grepple --daemon graph callers --symbol Symbol .`; focused `callees` is also supported. On a miss the CLI builds locally and publishes the report; an unavailable worker falls back to direct analysis. Search, GritQL, and remote requests do not use the worker.

The worker's single authenticated descriptor lives at `~/.grepple/cache/daemon.json`, or under an absolute, private `GREPPLE_CACHE_DIR` override. It uses a random bearer token and loopback-only TCP HTTP, with no platform-specific socket API; Unix enforces directory/file permissions and Windows relies on user-profile ACLs. Clients validate protocol, build identity, and listener address. Treat other processes with access to your user profile as trusted. Navigation caches remain per-repository at `~/.grepple/cache/<repository-id>/navigation` by default; an explicit `GREPPLE_NAVIGATION_CACHE_DIR` (including empty, disabling that cache) wins. Existing checkout-local caches are not migrated; output artifacts are separate.

The worker retains at most **four repository roots** and **eight report variants per root** under a **64 MiB serialized-report budget**, evicting least-recently-used entries or roots. Graph traversals are keyed by direction, depth, selector, and filters; resolve stores matching declarations, not the full graph, and rebuilds follow-up commands and output metadata for each invocation. Every request fingerprints the CLI-selected source paths, bytes, scope limit, and relevant ancestor configuration for its root. Changes cause a miss; reports built locally are revalidated by the worker before publication. Outside-root paths and symlink escapes are rejected; the CLI falls back to direct analysis. The cache is best effort, not a snapshot or substitute for current source accounting; restart the worker after upgrading/rebuilding Grepple. No repositories are indexed on startup.

Single diagnostic cold/warm-ish runs with one shared worker serving both the Pi checkout and a Go-heavy backend checkout (seconds; worker startup excluded; each worker's first request came after a direct run warmed the navigation cache):

| Full local request | Direct first / warmed | Worker miss / hit |
| --- | ---: | ---: |
| Pi `architecture directory --json .` | 18.03 / 3.62 | 3.73 / 0.33 |
| Go backend `architecture directory --json .` | 2.25 / 0.52 | 1.83 / 0.46 |
| Pi `graph resolve --symbol parseSkillBlock --json .` | 20.17 / 3.02 | 3.11 / 0.09 |
| Pi `graph callers --at packages/coding-agent/src/core/agent-session.ts:251 --depth 1 --json .` | 5.34 / 5.59 | 5.90 / 0.11 |
| Go backend `graph resolve --symbol WalkTree --json .` | 2.19 / 0.37 | 1.50 / 0.39 |
| Go backend `graph callers --symbol WalkTree --depth 1 --json .` | 0.40 / 0.34 | 1.60 / 0.38 |

Full JSON matched byte-for-byte in direct and worker modes for the sampled architecture and graph commands on both checkouts; neither checkout changed. These are one-run diagnostics, not latency guarantees. A worker miss adds request/publication overhead and can be slower than warm direct analysis; the cached report is lost on worker exit. The warmed speedup is substantial on the TypeScript-heavy Pi checkout, while this small Go backend did not benefit from the extra request round trip.

To repeat the comparison on another checkout, build both binaries, start `greppled` in a separate terminal (it need not run in that checkout), then time identical commands in the checkout and repeat each for warm behavior:

```sh
time grepple --no-spill architecture directory --json . > /tmp/grepple-direct.json
time grepple --no-spill --daemon architecture directory --json . > /tmp/grepple-daemon.json
cmp /tmp/grepple-direct.json /tmp/grepple-daemon.json
```

Use distinct output filenames per checkout, identical cache settings, and verify the worker is running: `--daemon` intentionally falls back to direct analysis if unavailable. Record the repository revision and binary identity. CLI timing excludes worker startup; the worker's first request includes local report construction on the client, while later requests reuse its in-memory report.
