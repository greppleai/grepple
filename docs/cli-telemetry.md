# Optional CLI tracing (local SigNoz)

CLI traces are **off by default**. In your user-owned `~/.grepple/settings.json`
(or an isolated file selected by `GREPPLE_SETTINGS=/path/to/settings.json`), add
these fields alongside any existing user settings, not in repository config:

```json
{
  "telemetry": {
    "enabled": true,
    "trace_endpoint": "http://127.0.0.1:4318/v1/traces",
    "trace_sample_ratio": 1
  }
}
```

Use this endpoint only when a trusted OTLP/HTTP collector is listening on the
**CLI host's** loopback port 4318 (for example an isolated local SigNoz collector
or forwarding agent); the backend container's own `127.0.0.1:4318` is a different
network namespace. This procedure does not start SigNoz or change live collector
settings. Only exact `/v1/traces` HTTPS URLs or explicit loopback HTTP are valid;
URLs with credentials, query or fragment are rejected. Do not put the
router/shard `GREPPLE_TRACE_PROPAGATION_KEY`, tokens or collector API credentials
in this telemetry object. Missing `telemetry` or `enabled:false` sends no traces.
An omitted `trace_sample_ratio` defaults to 1 when enabled; use a fraction from
0 to 1 to reduce volume. Unsampled invocations have no exported spans, and
sampling is inherited by trusted router and shards.

Run a CLI command normally, e.g. `grepple repos --server http://127.0.0.1:8787`.
In SigNoz Traces, filter `service.name=grepple-cli`, select an actual `cli.invocation` and inspect its `cli.command` child. For example, narrow command spans by `cli.command.name=search`, `cli.command.option.files=true` or `cli.command.option.outline=true`, then inspect the same trace's `cli.search.local` numeric `cli.files.attempted`, `cli.files.read`, and `cli.files.bytes_read`. Compare `cli.search.scan` wall time with its containing local-search span; `cli.search.parse` covers only the basic structural-segment batch. A flag Boolean reports effective parsed selection, not literal spelling. An omitted attribute means *not observed/not applicable*, not zero; `cli.results.count` is a pre-render selection, **not** delivered output items. There is no `cli.results.emitted` field. Use `cli.output_spill` on the same trace to filter by `cli.output.mode=inline|spilled` and inspect `cli.output.stdout_bytes` (bytes committed to stdout), `cli.output.logical_bytes` (captured command output after renderer truncation), and, for spilled output, `cli.output.artifact_bytes` (newly persisted artifact bytes, zero on content-addressed reuse). Spilled stdout is only a descriptor, not the artifact contents; do **not** sum descriptor and artifact bytes as one stdout total.

The trace shape is `cli.invocation` -> `cli.command` (work and optional real HTTP CLIENT requests), followed by sibling `cli.output_spill` (copy/persist only). A preparation error has no command/finalization span; artifacts-clean bypass has no finalization span or mode. Local search and outline have one `cli.search.local` child; their initial `cli.sources.discover`, content `cli.search.scan`, and applicable structural `cli.search.parse` are bounded children. Outline's streamed scan includes writes inside its `cli.render`: the nested **wall times overlap** and must not be added as CPU time. Local graph has one `cli.sources.resolve`, then `cli.navigation.execute` with an optional actual daemon `cli.navigation.cache_lookup` (`hit|miss`), fallback `cli.navigation.read`, a cold **resolve-only** `cli.navigation.build`, and local `cli.render`. Warm snapshot queries still read source files; snapshots are not daemon hits. No universally truthful graph-query build/query split exists. Remote-only operations have HTTP CLIENT spans and render but no local work counters; a network page is not a local scan. Search `--at`, stdin, listing and scope/related paths do not claim unsupported scan/parser stages or complete parser totals. Exactly measured zeros are distinct from missing fields. Spans use SDK start/end wall timestamps, fixed names and enum attributes; at most 10 bounded whole-command phases are emitted independently of file count, result count or graph depth. See [cli-usage-telemetry-contract.md](cli-usage-telemetry-contract.md) for units and omissions and [cli-boundary-tracing.md](cli-boundary-tracing.md) for source-backed seams.
Copy the selected invocation's trace ID and search the same ID for `grepple-router` and `grepple-shard`. When an operator has *separately* enabled backend telemetry, required real authentication, and router-only `--trust-local-cli-traces`, an eligible `/public` request from a **direct TCP loopback peer** can have a child `cli.http.*` -> `router.cli_request` -> router operation -> signed shard branches. The router's outer pre-auth HTTP SERVER span is intentionally a separate root. A remote router, Docker host-port forward seen as a bridge peer, proxy of untrusted traffic, missing auth or non-Public scope **cannot** be claimed as joined. Even without joining, local CLI and backend traces can be inspected separately; never enable trust merely to make a graph look connected. See the backend paths `grepple-backend/docs/tracing.md` (trust policy) and `grepple-backend/docs/observability.md` (SigNoz guidance) in the sibling checkout for the operator's corresponding configuration.

The CLI uses a private provider per invocation, fixed safe span categories and
bounded export/shutdown; it does not export URL, host, query, source, repository
path, credentials, error text, arbitrary resource attributes, tracestate or
baggage. Collector outages should not change command results. Some commands
(e.g. setup/help, local daemon-only work, OAuth bootstrap) do not propagate a
backend parent. Same-origin traced redirects are allowed; cross-origin traced
redirects are stopped. Coverage is high-level, not CPU profiling or complete
background indexing/parse timings.

**Rollback:** set `"enabled": false` or remove just `telemetry` from your own
settings file; separately turn off the operator-only router ingress flag (or
restart with it disabled). Do not change a running deployment, its credentials,
collector configuration or SigNoz volumes just for this recipe. No live SigNoz
verification is claimed by the integration tests; they use isolated OTLP
captures and temporary settings.
