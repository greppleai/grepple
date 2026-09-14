---
name: architecture-boundary-review
description: Use when deciding whether local code should move, split, merge, gain a facade, or when reviewing coupling, responsibility spread, dependency leakage, or package boundaries. Combines Grepple architecture, boundary signals, and focused call evidence; findings are hypotheses, not violations.
---

# Architecture boundary review with Grepple

A clean directory diagram can hide behavioral coupling; a broad type can be a healthy facade. Review ownership, behavior, and representation separately.

## Workflow

1. Establish the source universe, then inspect physical ownership and strongly resolved cross-directory evidence:
   ```bash
   grepple sources explain --compact SCOPE
   grepple architecture directory --depth 2 --compact .
   grepple architecture why FROM TO --compact SCOPE
   ```
2. Find repeated behavior and concrete-type spread:
   ```bash
   grepple boundaries path/to/scope
grepple boundaries --json path/to/scope
grepple boundaries --policy .grepple/boundary-policy.json --json path/to/scope
   ```
   Human output is triage; JSON is complete but may be delivered through a `grepple-artifact-v1` descriptor. Inspect the descriptor and retrieve only relevant artifact ranges.
3. Inspect each high-value evidence location with `grepple --at`. Do not recommend a move from counts alone.
4. Test the proposed boundary from both sides with focused callers/callees:
   ```bash
   grepple graph callers --at owner/file.go:LINE --depth 2 --compact SCOPE
   grepple graph callees --at consumer/file.go:LINE --depth 2 --compact SCOPE
   ```
5. Verify public signatures and relevant directory relations in exact source before claiming API leakage.

## Interpretation rules

- Use `origin`, `spread`, `containment`, usage `surface`, `risk`, and `reasons` for triage. The compatibility `External` field only means an import path was recorded.
- Risk is heuristic: it prioritizes third-party public/production spread and down-ranks standard-library, test-only, and package-internal local use.
- Wide use is not automatically bad. Treat `escaped` containment and facade bypass as repository-policy evidence; without a loaded policy, cross-boundary containment remains `unknown`.
- Public third-party representation and confirmed facade bypass deserve more attention than broad private use of a project-owned abstraction.
- Workflow candidates show repeated co-usage/order across owner-file boundaries. Check `signals`: utility, declarative configuration, lifecycle, test-framework, and adapter roles lower priority without hiding evidence.
- Boundary analysis is syntax-based and heuristic. Candidate edges and unresolved ownership must lower confidence.
- Compare production and test reach; never inflate production risk with test-framework spread. Use `--production-only` only when the review question intentionally excludes tests and fixtures.

## Required conclusion shape

Report: current owner, consumers, static package direction, repeated workflow/type evidence, public exposure, likely origin/containment, ambiguities, and one source-backed recommendation. Use “signal” or “lead” until source inspection confirms a violation.
