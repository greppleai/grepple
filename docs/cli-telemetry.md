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
In SigNoz Traces, filter `service.name=grepple-cli`, select an actual `cli.invocation` and inspect its `cli.command` child. After the command finishes, a sibling `cli.output_spill` measures **only final output copying or artifact persistence**, not the command runtime or a guaranteed artifact spill; it is absent for the artifacts-clean bypass. `cli.command.name` is a fixed parsed-command category, never the argv. Local search has at most one `cli.search.local` child with one initial `cli.sources.discover` grandchild; output is timed by one `cli.render` command child. Local graph has one `cli.sources.resolve` command child, followed by `cli.navigation.execute`, containing an optional real daemon `cli.navigation.cache_lookup` (fixed `hit|miss`) and one nested `cli.render`. Remote-only search/graph have HTTP CLIENT spans for real requests, then one render phase, but no local scan/graph phases. An error before output has no render phase. At most 10 new whole-command phases are allowed per invocation, independent of file/result/depth count; HTTP spans remain per real request. Phase attributes contain only fixed enums and already-known numeric counts. A stored graph snapshot is **not** labeled as a cache hit. Outline's render operation includes its own discovery/parse. Per-file/glob/result/worker/page internals and background indexing remain uninstrumented; duration is not a CPU profile.

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
