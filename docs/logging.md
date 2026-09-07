# Logging guidelines

grepple uses [zap](https://github.com/uber-go/zap) for structured logging in the
router. The guiding principle is **log as little as possible**: production should
be quiet, and every line that survives must be worth reading during an incident.

## Levels

| Level   | Use for | Emitted in production? |
|---------|---------|------------------------|
| `error` | Something failed and needs attention (a reconcile source failed, a dependency is unreachable in a way that loses data). | **Yes** |
| `warn`  | Recoverable anomalies worth noticing. Use sparingly. | Only if explicitly enabled |
| `info`  | Lifecycle and periodic aggregate stats (startup, one reconcile summary per cycle), plus one line per search with per-shard/phase timings for latency diagnosis. | **No** (enable to investigate) |
| `debug` | Developer detail. | **No** |

The level is controlled by `GREPPLE_LOG_LEVEL` (`debug`\|`info`\|`warn`\|`error`)
and **defaults to `error`**. Output is **always JSON** so logs stay
machine-parseable wherever they are shipped.

In production the default means: **errors only, no info or debug**. To
investigate (for example, to see repo-discovery stats) temporarily set
`GREPPLE_LOG_LEVEL=info` — do not leave it on.

## Rules

1. **Errors are the only thing logged by default.** If you log at `error`, it
   must be actionable. Do not log an error and then also return it up the stack
   where it will be logged again — log it once, at the boundary that handles it.
2. **Never log per-item in a loop.** Aggregate instead. Discovering 900
   repositories is **one** log line with counts, not 900 lines. Emitting a line
   per repo/file/request is prohibited — it drowns real signal and costs money.
3. **One summary per cycle.** Periodic work (reconcile, rebalance) logs a single
   structured summary at `info` with counts, plus separate `error` lines for the
   handful of failures.
4. **Prefer structured fields over string interpolation.** `zap.Int("desired",
   n)` not `fmt.Sprintf("desired=%d", n)`. Fields are queryable; sentences are
   not.
5. **No secrets, tokens, or full request bodies.** Never log installation
   tokens, private keys, `Authorization` headers, or webhook payloads. Log
   identifiers (org, installation id) not credentials.
6. **Don't log the happy path.** A successful search, a successful clone, a
   healthy probe — none of these are logged. Silence means healthy.
7. **Bound cardinality.** Avoid high-cardinality fields (per-repo names, per-user
   ids) in messages that fire frequently; they blow up log indexes.
8. **Expose stats over HTTP, not logs, when a caller wants them.** `/reconcile`
   returns `discovered/archived/disabled/otherOrg/desired/present/queued/
   deferred`. Reach for an endpoint or metric before adding an info log.

## Examples

Good — one aggregate summary, structured, at info (suppressed in prod):

```go
s.log().Info("reconcile",
    zap.Int("discovered", discovered),
    zap.Int("archived", archived),
    zap.Int("disabled", disabled),
    zap.Int("desired", len(desired)),
    zap.Int("queued", len(queued)),
    zap.Int("errors", len(errorsOut)),
)
```

Good — predefined-grep (rule) evaluation follows the same discipline: the search
path it reuses is silenced (reqId `rule:*`), and the rule worker emits **one**
aggregate line per job instead of one per repo/rule:

```go
rs.log.Info("rules evaluated", zap.String("repo", repo), zap.Int("rules", n),
    zap.Int64("ms", elapsedMs))
```

Good — an actionable error, logged once at the boundary:

```go
s.log().Error("reconcile source failed", zap.String("detail", detail))
```

Bad — per-item logging and string interpolation:

```go
for _, r := range repos {
    log.Printf("indexing repo %s", r.FullName) // NO: one line per repo
}
```

## Getting a logger

The router builds its logger in `Run` and stores it on `routerState`. Use
`s.log()` (not `s.logger` directly) so code paths without a configured logger
(tests) get a safe no-op logger instead of a nil panic.
