# Agent utility metrics

Grepple records privacy-safe agent activity in a local, provider-neutral JSONL journal. The journal separates evidence Grepple can observe from normalized evidence supplied by an automation harness. Reports and comparisons are deterministic transformations of that explicit evidence.

## Quick start

Start one repository-scoped run:

```sh
grepple metrics start --task issue-123 --cohort grepple
# Run normal Grepple commands and the rest of the task.
grepple metrics status
grepple metrics end --outcome success
```

`start` prints the generated run ID and journal path. Use `--run ID`, `--event-id ID`, and `--at RFC3339` when a harness needs explicit, reproducible correlation. `start` also accepts `--repository` and `--revision`; otherwise Grepple derives them from the current checkout.

While a run is active, Grepple commands append only command name/mode, success, and duration. Metrics commands do not record themselves. Recording is best-effort and does not alter a normal command's output or exit behavior.

## Supply normalized agent evidence

A provider or automation harness can append evidence with `metrics record`. It may target the active run or pass `--run ID` explicitly.

```sh
grepple metrics record --event assistant --data \
  '{"turn":1,"provider":"provider","model":"model","thinkingLevel":"medium","usage":{"input":120,"output":30,"cacheRead":0,"cacheWrite":0,"totalTokens":150,"cost":0.002}}'

grepple metrics record --event tool_call --data \
  '{"callId":"call-1","tool":"read","category":"read","argumentShape":"resource-range","resourceId":"resource-1","offset":1,"limit":80,"turn":1,"greppleMode":""}'

grepple metrics record --event tool_result --data \
  '{"callId":"call-1","success":true,"bytes":2400,"lines":80,"zeroResult":false,"addedLines":null,"removedLines":null,"beforeFingerprint":"","afterFingerprint":"","testOutcome":""}'

grepple metrics record --event compaction --data '{}'
```

Supported normalized events are:

- `assistant`: turn number, provider/model labels, thinking level, and provider-reported usage.
- `tool_call`: opaque call ID, tool/category, argument shape, optional opaque resource ID, optional numeric range, turn, and Grepple mode.
- `tool_result`: call ID, success, output byte/line counts, zero-result flag, optional edit counts/fingerprints, and `pass`, `fail`, or `unknown` test outcome.
- `compaction`: an empty data object.

Unknown fields and invalid values are rejected. Use identifiers and labels that contain no secrets. Resource IDs may correlate calls without revealing content-bearing paths.

End a run with an explicit evaluation:

```sh
grepple metrics end --outcome success \
  --score 4.5 --human-interventions 0 --regressions 0 \
  --first-edit-survived true --rubric acceptance-v1
```

Outcomes are `success`, `failure`, `abandoned`, or `unknown`. Optional evaluation values accept `-` for unavailable evidence.

## Reports and comparisons

Human report:

```sh
grepple metrics report --input ~/.grepple/metrics --group-by cohort
```

Stable JSON or CSV:

```sh
grepple metrics report --input ./evidence --format json > report.json
grepple metrics report --input ./evidence --format csv > runs.csv
```

Explicit cohort comparison:

```sh
grepple metrics compare --input ./evidence \
  --baseline control --target grepple --format json
```

Comparison deltas are `target - baseline`; percent is unavailable when the baseline is zero. They are descriptive and make no statistical-significance claim. Grouping dimensions are `cohort`, `observed`, `model`, `repository`, and `task`. Filters include repository, model, task, cohort, completion, and explicit RFC3339 time bounds.

Given identical journals and options, JSON, CSV, and human output are stable. Runs and groups are sorted, comparison endpoints are explicit, and `generatedAt` comes from the latest run evidence rather than the current clock.

## Evidence availability

Grepple derives only metrics supported by journal evidence:

- Assistant records provide turns, token/cache/cost usage, provider, model, and thinking level.
- Tool call/result pairs provide categories, outcomes, result volume, repetition, read overlap, navigation conversion, edit churn, revert proxies, test/fix cycles, and milestones.
- Automatically observed Grepple commands provide Grepple use, mode, success, and first useful evidence. They do **not** provide model usage, agent turns, tool output volume, or edit/test details.
- `run_end` provides completion and explicit outcome/evaluation values.

Unavailable evidence is listed in each run's `missing` array; it is not represented as an observed zero. Retries and semantic relevance remain unavailable in journal v1. Incomplete runs remain reportable unless `--complete` is used.

## Controlled cohorts

Use the same task IDs and pinned repository revisions for control and target runs. Hold provider/model, thinking level, instructions, context limits, time limits, and evaluation rubric constant. Assign the intended cohort explicitly:

```sh
grepple metrics start --task task-42 --cohort control --run task-42-control
grepple metrics start --task task-42 --cohort grepple --run task-42-grepple
```

Run those commands separately; only one active run exists per repository root. Keep collection enabled in both cohorts. Disable Grepple assistance—not measurement—for the control cohort, then compare the explicit `control` and `grepple` groups.

## Privacy, storage, and deletion

The journal never stores prompts, responses, source text, tool output, raw search queries, raw shell commands, command arguments, or content-bearing paths. It stores normalized metadata: opaque identifiers, labels, timestamps, counts, booleans, outcomes, and optional fingerprints.

The default root is `~/.grepple/metrics`; set `GREPPLE_METRICS_DIR` to override it. Journals are under `runs/`, and repository-scoped active-state files are under `active/`. Directories are mode `0700` and files are mode `0600`. Appends use an advisory file lock, write complete JSONL records, reject duplicate event IDs, sync before returning, and limit each record to 1 MiB. Readers reject malformed, truncated, unsupported, or duplicate records.

To delete collected evidence, remove the corresponding `runs/<run-id>.jsonl`. If that run is active, end it first or remove its repository-scoped file under `active/`. Grepple sends no metrics journal data to a backend.
