---
name: architecture-boundary-review
description: Use when deciding whether local code should move, split, merge, gain a facade, or when reviewing coupling and package boundaries. Treat directory signals as leads until verified in source.
---

# Architecture boundary review with Grepple

## Investigate one proposed boundary

1. Establish the source universe with `grepple sources explain SCOPE`, then inspect bounded physical relations with `grepple architecture directory --relations SCOPE` and `grepple architecture why FROM TO SCOPE`. Directory ownership does not establish package intent.
2. Search relevant files for repeated owner usage and concrete-type references. Inspect exact evidence with `grepple --at PATH:START-END`; verify public signatures before claiming API leakage.
3. Test the proposed boundary from both sides: `grepple graph callers --at owner/file.go:LINE --depth 2 SCOPE` and `grepple graph callees --at consumer/file.go:LINE --depth 2 SCOPE`. Check candidates and omissions; static edges do not establish runtime behavior.

## Decide and report

Wide use is not automatically bad. Compare production and test reach; use `--production-only` only for an explicitly production-scoped decision. Report current owner and consumers, static dependency direction, source-backed workflow/type evidence, public exposure, ambiguities or source gaps, and one recommendation with a cited range. If unresolved, call it a lead and state the next inspection step.
