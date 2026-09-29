---
name: architecture-diagram-workflow
description: Use for source-linked directory dependency Mermaid diagrams. For type shapes and call flow, use exact source and graph navigation rather than removed focused extraction commands.
---

# Directory diagrams with Grepple

Use `grepple architecture directory --mermaid --depth 2 --max-nodes 80 --output /tmp/architecture.mmd SCOPE` for a scoped directory dependency diagram. `--relations` provides a smaller text projection. Inspect the underlying `--json` relation evidence and exact source before making architectural claims. For callable relationships, run `grepple graph callers --at PATH:LINE SCOPE` or `grepple graph callees --at PATH:LINE SCOPE`; for type shapes, use `grepple --outline PATH` and `grepple --at PATH:LINE`. Focused structure/flow diagram generation is no longer exposed through the CLI. Static edges do not prove runtime flow. Keep one-off diagrams in `/tmp` unless the repository owns them.
