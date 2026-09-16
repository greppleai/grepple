# Agent utility metrics

Grepple is a read-only analyzer for provider-neutral agent-utility journals. An external automation harness produces `grepple-metrics-event-v1` JSONL; Grepple validates only the files or directories named with `--input`, derives metrics from that evidence, and emits deterministic reports or comparisons.

Grepple does not start or end runs, observe commands automatically, write journals, maintain active state, choose a default metrics directory, or infer evidence from the current repository, Git, environment, home directory, working directory, or wall clock.

## Analyze explicit JSONL

Every report and comparison requires at least one `--input`. The option is repeatable and accepts a `.jsonl` file or a directory searched recursively for `.jsonl` files.

```sh
# Human-readable report.
grepple metrics report --input ./evidence/run-a.jsonl --group-by cohort

# Stable JSON or CSV export.
grepple metrics report --input ./evidence --format json > report.json
grepple metrics report --input ./evidence --format csv > runs.csv

# Explicit target-minus-baseline comparison.
grepple metrics compare --input ./control --input ./target \
  --baseline control --target grepple --format json
```

Grouping dimensions are `cohort`, `observed`, `model`, `repository`, and `task`. Filters include repository, model, task, cohort, complete runs, and explicit RFC3339 time bounds. Comparison endpoints are required and must identify different groups. Deltas are `target - baseline`; percentages are unavailable when the baseline is zero. Comparisons are descriptive and make no statistical-significance claim.

Given identical journals and options, text, JSON, CSV, and comparison output are stable. Input paths, runs, and groups are sorted deterministically. `generatedAt` is the latest end timestamp present in the selected run evidence, not the current time.

## Version 1 event contract

Each non-empty line is one JSON object with this envelope:

```json
{"schema":"grepple-metrics-event-v1","eventId":"event-1","time":"2026-01-01T00:00:00Z","runId":"run-1","event":"run_start","data":{"taskId":"task-42","repository":"grepple","revision":"abc123","assignedCohort":"control"}}
```

Envelope fields are strict: `schema` must be `grepple-metrics-event-v1`; `eventId`, `time`, `runId`, `event`, and valid event-specific `data` are required. Unknown fields, unsupported schemas or event types, malformed/truncated JSON, duplicate event IDs, and invalid values are rejected.

Supported events:

- `run_start`: bounded `taskId` and `assignedCohort` labels, plus optional `repository` and `revision` labels.
- `assistant`: positive turn, provider/model/thinking-level labels, and non-negative provider-reported input, output, cache, total-token, and cost usage.
- `tool_call`: opaque call/tool identifiers, category, argument shape, optional opaque `resourceId`, optional numeric range, turn, and optional Grepple mode. Categories are `navigation`, `read`, `mutation`, `test`, `verification`, or `ambiguous`.
- `tool_result`: matching call ID, success, non-negative output byte/line counts, zero-result flag, optional edit counts and opaque before/after fingerprints, and optional `pass`, `fail`, or `unknown` test outcome.
- `command`: an externally observed opaque command name and Grepple mode, success, and non-negative duration. Grepple does not generate this event automatically.
- `compaction`: an empty data object.
- `run_end`: `success`, `failure`, `abandoned`, or `unknown`, with optional human-intervention count, evaluator score, opaque rubric ID, regression count, and first-edit-survived value.

A run must begin with exactly one `run_start`. Other run events follow it, and nothing may follow `run_end`. Tool results must correlate to supplied tool calls. See [`internal/metrics/testdata/comprehensive-v1.jsonl`](../internal/metrics/testdata/comprehensive-v1.jsonl) for a complete multi-cohort fixture.

## Producer responsibilities

The external producer owns collection, lifecycle correlation, safe storage, retention, concurrency control, and durable writing. It should:

1. Write complete JSON objects separated by newlines and never expose a partially written file to analysis.
2. Supply stable opaque run, event, call, resource, fingerprint, command, mode, and rubric identifiers.
3. Supply timestamps and all observed values explicitly; do not encode unavailable evidence as zero.
4. Preserve run lifecycle order and use unique event IDs within each journal.
5. Protect journals according to the producer's security and retention policy.
6. Exclude prompts, responses, source, tool output, raw queries, raw commands, command arguments, and content-bearing paths.

Grepple has no production journal-writing API and creates no lock sidecars. Analyze immutable snapshots or otherwise ensure the producer is not modifying an input while it is read.

## Validation bounds

Reading is fail-closed and bounded per journal:

- maximum JSONL record size: 1 MiB;
- maximum journal size: 64 MiB;
- maximum events per journal: 100,000;
- strict JSON envelope and event-data decoding;
- duplicate event-ID and lifecycle rejection.

Explicit directories are recursively searched only for `.jsonl` files. Discovered paths are deduplicated and sorted. A missing input, a non-JSONL file input, an unreadable path, or any invalid selected journal fails the command rather than being silently ignored.

## Evidence availability

Grepple derives only metrics supported by supplied events:

- Assistant events provide turns, token/cache/cost usage, provider, model, and thinking level.
- Tool call/result pairs provide categories, outcomes, result volume, repetition, read overlap, navigation conversion, edit churn, revert proxies, test/fix cycles, and milestones.
- Command events can provide externally observed Grepple use, mode, success, duration, and first useful evidence.
- `run_end` provides completion and explicit outcome/evaluation values.

Unavailable evidence is listed in each run's `missing` array and is never converted to an observed zero. Nullable report aggregates remain unavailable when no run in a group supplied the underlying evidence. Retries and semantic relevance are unavailable in journal v1. Incomplete runs remain reportable unless `--complete` is supplied.

## Controlled comparisons

Use the same task IDs and pinned repository revisions for baseline and target runs. Hold provider/model, thinking level, instructions, context limits, time limits, producer behavior, and evaluation rubric constant. Assign cohort labels in each externally generated `run_start`, then select them explicitly:

```sh
grepple metrics compare --input ./experiment.jsonl \
  --group-by cohort --baseline control --target grepple
```

Assigned cohort and observed Grepple use are separate dimensions. A producer should measure both cohorts consistently; absence of an event means unknown evidence, not proof that an activity did not occur.

## Privacy

The v1 schema is designed for normalized metadata: opaque identifiers, bounded labels, timestamps, counts, booleans, outcomes, and optional fingerprints. It has no fields for prompts, responses, source text, tool output, raw queries, raw commands, command arguments, or content-bearing paths, and unknown fields are rejected.

Privacy still depends on the producer choosing safe labels and opaque identifiers. Do not place sensitive or content-bearing values in `taskId`, `repository`, `revision`, provider/model labels, or any identifier. Grepple reads the explicitly supplied journals locally and does not send their contents to a backend.
