# Agent utility metrics

Grepple is a read-only analyzer for coding-agent utility journals. An external automation harness produces `grepple-metrics-event-v1` JSONL; Grepple validates only the files or directories named with `report --input`, derives metrics from that evidence, and emits deterministic reports grouped by the producing agent. Comparison reads two previously generated JSON reports and does not reparse their source journals.

Grepple does not start or end runs, observe commands automatically, write journals, maintain active state, choose a default metrics directory, or infer evidence from the current repository, Git, environment, home directory, working directory, or wall clock.

## Analyze explicit JSONL

`metrics report` requires at least one `--input`. The option is repeatable and accepts a `.jsonl` file or a directory searched recursively for `.jsonl` files. `metrics compare` instead requires one baseline and one target JSON report previously emitted by `metrics report --format json`.

```sh
# Human-readable report, always grouped by producing agent.
grepple metrics report --input ./evidence/run-a.jsonl

# Stable JSON or CSV output.
grepple metrics report --input ./without-grepple.jsonl --format json > baseline.json
grepple metrics report --input ./with-grepple.jsonl --format json > target.json
grepple metrics report --input ./evidence --format csv > runs.csv

# Target-minus-baseline comparison of two generated reports.
grepple metrics compare --baseline baseline.json --target target.json
```

`report` accepts only repeatable `--input` and `--format text|json|csv`. `compare` accepts only `--baseline`, `--target`, and `--format text|json|csv`; it rejects `--input`. Each comparison input must be a strict, canonical `grepple-agent-report-v1` JSON report containing exactly one agent group. The baseline and target may name the same coding agent, allowing separate runs with and without Grepple to be compared. Deltas are `target - baseline`; percentages are unavailable when the baseline is zero. Comparisons are descriptive and make no statistical-significance claim.

Given identical evidence and options, text, JSON, CSV, and comparison output are stable. Journal paths, runs, and agent groups are sorted deterministically. Report `generatedAt` is the latest end timestamp present in the supplied run evidence; comparison `generatedAt` is the later of its two report timestamps. Neither uses the current time.

## Version 1 event contract

Each non-empty line is one JSON object with this envelope:

```json
{"schema":"grepple-metrics-event-v1","eventId":"event-1","time":"2026-01-01T00:00:00Z","runId":"run-1","event":"run_start","data":{"agent":"pi"}}
```

Envelope fields are strict: `schema` must be `grepple-metrics-event-v1`; `eventId`, `time`, `runId`, `event`, and valid event-specific `data` are required. Unknown fields, unsupported schemas or event types, malformed or truncated JSON, duplicate event IDs, and invalid values are rejected.

Supported events:

- `run_start`: one required bounded `agent` label identifying the coding agent that produced the run, such as `pi` or `claude-code`.
- `assistant`: positive turn and non-negative reported input, output, cache, total-token, and cost usage.
- `tool_call`: opaque call/tool identifiers, category, argument shape, optional opaque `resourceId`, optional numeric range, turn, and optional Grepple mode. Categories are `navigation`, `read`, `mutation`, `test`, `verification`, or `ambiguous`.
- `tool_result`: matching call ID, success, non-negative output byte/line counts, zero-result flag, optional edit counts and opaque before/after fingerprints, and optional `pass`, `fail`, or `unknown` test outcome.
- `command`: an externally observed opaque command name and Grepple mode, success, and non-negative duration. Grepple does not generate this event automatically.
- `compaction`: an empty data object.
- `run_end`: `success`, `failure`, `abandoned`, or `unknown`, with optional human-intervention count, evaluator score, opaque rubric ID, regression count, and first-edit-survived value.

A run must begin with exactly one `run_start`. Other run events follow it, and nothing may follow `run_end`. Tool results must correlate to supplied tool calls. See [`internal/metrics/testdata/comprehensive-v1.jsonl`](../internal/metrics/testdata/comprehensive-v1.jsonl) for a complete two-agent fixture.

## Producer responsibilities

The external producer owns collection, lifecycle correlation, safe storage, retention, concurrency control, and durable writing. It should:

1. Write complete JSON objects separated by newlines and never expose a partially written file to analysis.
2. Supply a stable bounded agent name and stable opaque run, event, call, resource, fingerprint, command, mode, and rubric identifiers.
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

Explicit directories are recursively searched only for `.jsonl` files. Discovered paths are deduplicated and sorted. A missing input, a non-JSONL file input, an unreadable path, or any invalid selected journal fails the command rather than being silently ignored. Comparison report files are limited to 64 MiB and decoded strictly; unsupported schemas, unknown fields, trailing JSON values, and summaries that do not exactly match the contained runs are rejected.

## Evidence availability

Grepple derives only metrics supported by supplied events:

- Assistant events provide turns and token/cache/cost usage.
- Tool call/result pairs provide categories, outcomes, result volume, repetition, read overlap, navigation conversion, edit churn, revert proxies, test/fix cycles, and milestones.
- Command events can provide externally observed Grepple use, mode, success, duration, and first useful evidence.
- `run_end` provides completion and explicit outcome/evaluation values.

Unavailable evidence is listed in each run's `missing` array and is never converted to an observed zero. Nullable report aggregates remain unavailable when no run in an agent group supplied the underlying evidence. Retries and semantic relevance are unavailable in journal v1. Incomplete runs remain reportable as supplied evidence.

## Controlled comparisons

To compare two runs or run sets, generate one JSON report for each controlled condition. Hold agent version, instructions, context limits, time limits, producer behavior, evaluation rubric, and workload constant outside the journal. The producing agent may be the same in both reports:

```sh
grepple metrics report --input ./without-grepple.jsonl --format json > baseline.json
grepple metrics report --input ./with-grepple.jsonl --format json > target.json
grepple metrics compare --baseline baseline.json --target target.json
```

Comparison validates each report as canonical output and requires exactly one agent group per file. It uses only the report contents, so the original JSONL does not need to be available. Absence of evidence remains unknown, not proof that an activity did not occur.

## Privacy

The v1 schema is designed for normalized metadata: opaque identifiers, one bounded agent label, timestamps, counts, booleans, outcomes, and optional fingerprints. It has no fields for prompts, responses, source text, tool output, raw queries, raw commands, command arguments, or content-bearing paths, and unknown fields are rejected.

Privacy still depends on the producer choosing a safe agent label and opaque identifiers. Do not place sensitive or content-bearing values in any label or identifier. Grepple reads the explicitly supplied journals locally and does not send their contents to a backend.
