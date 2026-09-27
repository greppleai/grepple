---
name: architecture-boundary-review
description: Use when deciding whether local code should move, split, merge, or gain a facade. Combines bounded Grepple directory relations, focused source search, and call evidence; findings are hypotheses, not violations.
---

# Architecture boundary review with Grepple

A clean directory diagram can hide behavioral coupling; a broad type can be a healthy facade. Review ownership, behavior, and representation separately.

## Workflow

1. Establish the source universe with `grepple sources explain SCOPE`, then inspect a bounded `grepple architecture directory --relations SCOPE` report.
2. Search for repeated owner usage and concrete-type references in the relevant source files. Inspect evidence with `grepple --at PATH:LINE`; do not recommend a move from counts alone.
3. Test the proposed boundary from both sides with focused callers/callees:
   ```bash
   grepple graph callers --at owner/file.go:LINE --depth 2 SCOPE
   grepple graph callees --at consumer/file.go:LINE --depth 2 SCOPE
   ```
4. Verify public signatures and relevant directory relations in exact source before claiming API leakage.

## Interpretation rules

- Wide use is not automatically bad. Inspect public signatures, ownership, and source-backed call sites before proposing a boundary change.
- Compare production and test reach; never inflate production risk with test-framework spread. Use `--production-only` only when the question intentionally excludes tests and fixtures.

## Required conclusion shape

Report: current owner, consumers, static package direction, repeated workflow/type evidence, public exposure, likely origin/containment, ambiguities, and one source-backed recommendation. Use “signal” or “lead” until source inspection confirms a violation.
