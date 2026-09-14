---
name: architecture-lookup-discovery
description: Use to orient in an unfamiliar local repository or answer module/package ownership, dependency direction, public-surface, route, and entrypoint questions. Start from canonical .grepple summaries, then verify one exact source declaration. Do not use this skill alone for call impact or proposed boundary changes.
---

# Architecture lookup with Grepple

Architecture artifacts are an index, not proof of runtime behavior. Use them to identify the owner and smallest source range, then verify that source.

## Fast path

1. **Unknown owner:** inspect the bounded workspace summary.
   ```bash
   grepple extract summary workspace .
   ```
   If canonical artifacts exist, `.grepple/project.workspace/overview.mmd` is the stable module/package/route map.
2. **Known package:** inspect its bounded public surface.
   ```bash
   grepple extract summary package path/to/package
   ```
   Read `.grepple/<package>.package/overview.mmd` only when relation evidence is needed; use `structure.mmd` only for exhaustive fields and methods. Avoid the large manifest for orientation.
3. **Verify source:** follow the artifact's `PATH:START-END`.
   ```bash
   grepple --at path/to/file.go:40-80
   ```
4. **Only if behavior matters:** switch to `change-impact-analysis` instead of inferring behavior from package arrows or type relations.

## Trust rules

- Generated `.grepple/` files are read-only; never patch them manually.
- Package/import edges establish static ownership direction, not runtime invocation.
- Type relations establish declared shape, not object lifetime, side effects, or data flow.
- If the decision depends on a canonical artifact being current, validate it first:
  ```bash
  grepple extract check workspace .grepple/project.workspace
  grepple extract check package .grepple/<package>.package
  ```
  In this repository, use `make schema-check` for the aggregate gate.
- If no canonical bundle exists, use local file/outline discovery; do not invent a package boundary from directory names alone.

## Report only evidence

Preserve the owning module/package, relevant declaration range, static dependency/type relation, artifact freshness, and the next source location inspected. State uncertainty rather than filling missing behavior from the diagram.

Use `architecture-boundary-review` when deciding whether code should move, split, or sit behind a facade.
