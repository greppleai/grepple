---
name: architecture-boundary-review
description: Use when deciding whether local code should move, split, merge, gain a facade, or when reviewing coupling, responsibility spread, dependency leakage, or package boundaries. Combines Grepple architecture, boundary signals, and focused call evidence; findings are hypotheses, not violations.
---

# Architecture boundary review with Grepple

A clean directory diagram can hide behavioral coupling; a broad type can be a healthy facade. Review ownership, behavior, and representation separately.

## Workflow

1. Establish current ownership and dependency direction:
   ```bash
   grepple extract summary workspace .
   grepple extract summary package path/to/package
   ```
2. Find repeated behavior and concrete-type spread:
   ```bash
   grepple boundaries path/to/scope
   grepple boundaries --json path/to/scope
   ```
   Human output is triage; JSON is complete.
3. Inspect each high-value evidence location with `grepple --at`. Do not recommend a move from counts alone.
4. Test the proposed boundary from both sides with focused callers/callees:
   ```bash
   grepple graph callers --at owner/file.go:LINE --depth 2 --compact SCOPE
   grepple graph callees --at consumer/file.go:LINE --depth 2 --compact SCOPE
   ```
5. Verify public signatures and package relations in source or canonical package structure before claiming API leakage.

## Interpretation rules

- Current `External` means an import path was recorded; it does **not** distinguish standard library, first-party, and third-party dependencies.
- Wide use is not automatically bad. Test whether use is package-internal, cross-package, public API, test-only, or outside an intended containment zone.
- Public third-party representation and facade bypass deserve more attention than broad private use of a project-owned abstraction.
- Workflow candidates show repeated co-usage/order across owner-file boundaries. Configuration setup, lifecycle cleanup, tests, and utility hubs often produce benign repetition.
- Boundary analysis is syntax-based and heuristic. Candidate edges and unresolved ownership must lower confidence.
- Compare production and test reach; never inflate production risk with test-framework spread.

## Required conclusion shape

Report: current owner, consumers, static package direction, repeated workflow/type evidence, public exposure, likely origin/containment, ambiguities, and one source-backed recommendation. Use “signal” or “lead” until source inspection confirms a violation.
