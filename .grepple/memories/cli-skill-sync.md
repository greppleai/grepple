---
schema: grepple-memory-v1
name: cli-skill-sync
description: Review affected Grepple skills whenever internal/cli changes.
domainAreas:
    - cli
scopes:
    - internal/cli/
---
# Keep CLI-facing skills synchronized

When changing behavior, flags, help, search results, navigation, output contracts, or telemetry anywhere in `internal/cli/` (including subpackages):

1. Review the repository-owned Grepple skills under `.agents/skills/` and identify which instructions, command examples, or assumptions depend on the changed CLI behavior. Check associated public guides and `grepple examples` where relevant.
2. Update only affected skills and examples in the same change, or explicitly record why none needs updating. Do not rewrite unrelated skills.
3. If a bundled skill changes, update its entry and checksum in `internal/agentskills/catalog.json`, verify the bundle/install tests, and keep installed or plugin copies in sync through the supported setup workflow rather than blind copying.
4. Keep `.pi/skills/semantic-commits/` out of the distributable CLI/plugin skill catalog.

This is a review reminder, not evidence that every change requires a skill edit. The memory's search attachments/labels are best-effort; they cannot represent the entire `internal/cli/` directory or automatically enforce this check on every edit.
