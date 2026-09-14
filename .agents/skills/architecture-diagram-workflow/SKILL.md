---
name: architecture-diagram-workflow
description: Use when asked to create, update, inspect, or validate Mermaid architecture diagrams or canonical .grepple package/workspace artifacts. Uses source-linked Grepple extraction instead of hand-authored architectural guesses.
---

# Source-backed architecture diagrams with Grepple

Choose the projection by question; do not generate the largest diagram by default.

## Projection choice

- **Package/workspace orientation:** use `extract summary`; do not generate a diagram merely to read it.
- **Type, field, method, or dependency shape:** use focused `extract structure`.
- **Callable sequence or path:** use focused `extract flow`.
- **Stable repository documentation/CI:** generate or check the canonical package/workspace bundle. Canonical bundles are Go-specific.

## Focused workflow

Use a source location to avoid same-name ambiguity:

```bash
grepple extract structure --at path/to/model.ts:25 --source . --output /tmp/model.structure.mmd
grepple extract flow --at path/to/service.go:80 --source . --depth 2 --max-nodes 30 --output /tmp/service.flow.mmd
```

Start flow depth at 1–2. Narrow source roots before raising `--max-nodes`; a truncated diagram is not a complete architecture claim.

Validate any diagram used for a decision or committed documentation:

```bash
grepple extract check structure /tmp/model.structure.mmd .
grepple extract check flow /tmp/service.flow.mmd .
grepple extract check package .grepple/<package>.package
grepple extract check workspace .grepple/project.workspace
```

## Trust rules

- Prefer generated source locations and semantic metadata over labels guessed from names.
- A structure edge is declared shape; a flow edge is syntax-resolved navigation. Neither proves runtime data flow or dynamic dispatch.
- Treat candidate or truncated relations as incomplete and verify their `PATH:LINE` declarations.
- Never manually edit canonical `.grepple/` output. Regenerate through the repository's documented target and run its schema check.
- Keep one-off diagrams in `/tmp` unless the repository explicitly owns them.

Use `architecture-lookup-discovery` to find the owner first and `change-impact-analysis` when the decision is about callers rather than visualization.
