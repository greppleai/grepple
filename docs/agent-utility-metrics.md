# Agent utility metrics

Grepple's utility measurements compare coding-agent work with an assigned Grepple treatment against the same work without Grepple guidance. Speed is not sufficient evidence of utility: every efficiency result must be interpreted alongside completion, correctness, and rework.

## Measurement units

- **Session branch** is the full linear record sequence in a legacy Pi v1 JSONL session, or the selected root-to-leaf path in a Pi v2/v3 session. Abandoned sibling branches are excluded.
- **Logical run** begins at a `grepple-metrics-v1` `run_start` marker and ends at its matching `run_end`. Without markers, the analyzer reports one inferred run for the selected session branch and labels it inferred.
- **Task** is one or more logical runs sharing an explicitly assigned task ID. Paired experiments require a task ID. Unlabelled daily work can still be summarized, but is not treated as a controlled pair.

Reloading does not start a new logical run. Forking starts a distinct branch. If tree navigation leaves an active run's start marker behind, the recorder re-anchors the same explicit task and cohort as a fresh logical run on the selected branch; it never emits an orphaned end marker. A missing end marker produces an incomplete run rather than an assumed success.

## Exact metrics

Pi assistant messages provide the authoritative model usage. For each run, sum:

- input, output, cache-read, cache-write, and total tokens;
- input, output, cache-read, cache-write, and total monetary cost;
- assistant responses (turns), tool calls, successful and failed tool results;
- elapsed milliseconds from the first included event to the final included event;
- tool-result UTF-8 bytes and lines;
- distinct inspected and edited paths.

Provider token and cost values are exact as reported by the provider. Tool-result token estimates use `ceil(UTF-8 bytes / 4)` and are always named `estimatedToolResultTokens`; they are never added to provider token totals.

## Milestones

A milestone records elapsed time, one-based turn ordinal, calls before the milestone, inclusive call ordinal, and cumulative provider usage through the assistant response that emitted the milestone call.

- **First evidence** is the first successful navigation or read call with a non-empty normalized path that is later read or mutated at a related path. This is a downstream-use proxy, not proof that the model reasoned from the result.
- **First attempted mutation** is the first recognized `edit` or `write`-class call.
- **First successful mutation** is the first recognized mutation with a non-error result.
- **First passing test** is the first confidently classified test command with a successful result. Its elapsed time uses successful tool-result completion; its cumulative usage remains the usage through the assistant response that emitted the call.
- **Completion** is the matching run-end annotation. Inferred or incomplete runs have no completion milestone and retain an unknown outcome unless an explicit end marker says otherwise.

“Calls to first edit” means calls before the attempted mutation. `inclusiveCall` is also emitted to remove ambiguity. “Tokens to first edit” includes the complete assistant response that generated the mutation call. Parallel calls retain assistant source order, not completion order.

Shell commands that might mutate files or run tests but cannot be classified confidently set the corresponding observability reason. They do not silently count as non-mutations or non-tests.

## Derived metrics

Derived fields are explicitly marked as exact, estimated, or proxy-based:

- search-to-read and search-to-edit conversion by non-empty normalized related paths; pathless shell navigation is not treated as a wildcard;
- repeated call count from identical tool name and privacy-safe argument shape;
- redundant reads of the same normalized path and effective range, reset when that path is mutated;
- inspected-to-edited file ratio;
- edit operation count and added/removed line churn from current edit-result metrics when available, with legacy edit arguments retained as a compatibility source; unavailable exact churn adds `edit_churn` to `missing`;
- replacement/revert proxy from current unified edit-result patches or compatible legacy replacement arguments;
- test/fix cycles between failed and passing verification calls;
- first-edit survival when the extension records an observation at run end;
- tokens, cost, and elapsed time per successful task.

Semantic relevance, correctness, regressions, and historical final-file state cannot be reconstructed from session structure alone. They remain unavailable unless supplied by an annotation or evaluator.

## Treatment and outcomes

`assignedCohort` is experiment intent (`grepple`, `control`, or a caller-defined label). `observedGreppleUse` is derived independently from confidently recognized Grepple invocations. Reports never substitute one for the other. This exposes treatment non-compliance and incidental use.

Task annotations may provide:

- outcome: `success`, `failure`, or `abandoned`;
- human intervention count;
- evaluator score and optional rubric name;
- regression count;
- first-edit survival;
- repository revision and task ID.

Unknown values remain null with a reason. They are not converted to zero. Success-normalized metrics exclude tasks without a success/failure outcome and report the denominator.

## Privacy

The analyzer reads existing Pi session files locally. The metrics extension adds only versioned custom entries and must not copy prompt text, model responses, source code, tool output, command strings, edit text, or file contents into those entries. It stores identifiers, enums, counts, timestamps, booleans, numeric scores, and optional repository revision. User-provided task IDs and rubric names should not contain secrets.

Human reports are bounded. Machine exports contain aggregates and privacy-safe run metadata, not message or tool content. Deleting the source Pi session deletes its embedded metric annotations.

## Install and record

This repository is a Pi package: `package.json` exposes the recorder extension and the Grepple skills. Install a reviewed, pinned tag or commit globally, or install the current checkout project-locally:

```sh
pi install git:github.com/greppleai/grepple@<tag-or-commit>
pi install -l .
```

Use `pi update <package-source>` to reconcile an installed package. Pinned Git references do not move; install the new tag or commit explicitly when upgrading. `pi list` shows active package sources.

The recorder is passive by default. Start a labelled task explicitly:

```text
/metrics-start TASK_ID grepple
/metrics-status
/metrics-end success 4.5 0 0 true unit-rubric
```

The end arguments are outcome, optional evaluator score, human-intervention count, regression count, first-edit survival, and an optional opaque rubric name. Use `-` for each unknown optional value. Outcomes are `success`, `failure`, or `abandoned`. Task IDs and rubric names are persisted, so use opaque values without prompt text or secrets.

For automatic one-prompt logical runs, opt in when starting Pi:

```sh
pi --grepple-metrics --grepple-metrics-cohort grepple
```

Each automatic run ends with an unknown outcome; use explicit commands when evaluated outcomes are required. Explicit tasks survive reload/resume. A fork creates a distinct logical-run ID while retaining the task and cohort labels. Shutdown writes a content-free pause marker while leaving the task incomplete, rather than inventing an outcome.

For a control run, retain the extension but disable package skills with Pi package filtering in project settings:

```json
{
  "packages": [
    {
      "source": "git:github.com/greppleai/grepple@<tag-or-commit>",
      "extensions": [".pi/extensions/agent-metrics.ts"],
      "skills": []
    }
  ]
}
```

Then run Pi with `--grepple-metrics --grepple-metrics-cohort control`. Also remove Grepple-specific system guidance and tools from the control environment; disabling collection would confound the comparison.

Annotations stay in Pi's local session JSONL alongside the source events. The extension makes no network request. Retention follows Pi session retention. Delete the corresponding session JSONL to delete its annotations, and avoid distributing raw sessions because their pre-existing messages and tool records are not content-free.
## Comparison protocol

For a paired A/B comparison:

1. Pin the repository revision, model/provider, thinking level, system instructions, context window, task prompt, time limit, and completion criteria.
2. Assign each task a stable task ID. Randomize whether its first attempt is Grepple or control; reverse ordering across tasks to reduce learning effects.
3. Keep the metrics recorder enabled in both cohorts. In control runs disable Grepple skills/guidance and do not invoke Grepple; do not disable the recorder.
4. Use fresh sessions and equivalent caches unless cache state is itself under study.
5. Record outcome through tests plus a blinded evaluator whenever possible. Record human interventions.
6. Compare assigned cohorts first (intention-to-treat), then report observed-use compliance separately.
7. Report sample size, missingness, median, p50/p90, mean, absolute delta, and percentage delta. Do not claim significance from the summary alone.

Primary measures are success rate, total cost and elapsed time per successful task, and final evaluator score. Secondary measures are total tokens, calls, context volume, milestone speed, navigation efficiency, and rework. First-edit speed must always be shown with mutation success, first passing test, and rework.

## Known limitations

- Pi sessions expose provider usage per assistant response, not the exact token offset inside a response at which a tool call was generated.
- Generic shell commands can hide Grepple, tests, or file mutations. Ambiguous commands reduce observability.
- Tool-result byte-based token estimates vary by tokenizer.
- A path conversion is correlation, not proof of relevance.
- Parallel tool completion order differs from source order.
- Results across different models, revisions, prompts, cache states, or evaluators are not controlled comparisons.
