---
name: architecture-diagram-workflow
description: Use for source-linked directory dependency Mermaid diagrams. Focused type and call-flow extraction is no longer a CLI command; use exact source and graph navigation for those questions.
---

# Directory diagrams with Grepple

Use `grepple architecture directory --mermaid --depth 2 --max-nodes 80 --output /tmp/architecture.mmd SCOPE` for a scoped directory dependency diagram. `--relations` provides a smaller text projection. Inspect the underlying `--json` relation evidence and exact source before making architectural claims. For callable relationships, use `grepple graph callers|callees --at PATH:LINE`; for type shapes, use `grepple --outline PATH` and `--at`. Focused source/flow diagram generation is no longer exposed through the CLI. Static edges do not prove runtime flow.
