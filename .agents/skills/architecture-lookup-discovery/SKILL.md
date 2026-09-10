---
name: architecture-lookup-discovery
description: "Use for fast architectural lookup and discovery in a local checkout: orienting in an unfamiliar project, finding package ownership and boundaries, locating exact declarations, understanding dependencies and call paths, or assessing change impact. Start with canonical .grepple architecture artifacts, then narrow into source with Grepple navigation instead of reading directories or files broadly."
---

# Architecture lookup and discovery with Grepple

Use Grepple's generated architecture as an index into the code, not as documentation to read exhaustively. The fast path is **workspace → package overview → exact source range → related code**. This avoids repeated directory listing, text search, and whole-file reads.

## Why this is fast

- Workspace and package overviews compress thousands of source lines into stable ownership and dependency maps.
- Diagram nodes carry `PATH:START-END`, so architecture facts lead directly to declarations.
- Package structure diagrams retain exact members and relations when an overview is intentionally compact.
- `--at`, `--related`, and `--follow-related` turn one known location into bounded implementation and call-neighborhood context.
- Focused structure and flow extraction answer graph-shaped questions without loading an entire package model.

Do not infer implementation behavior from diagrams alone. Use them to choose the smallest relevant source location, then verify in source.

## Artifact hierarchy

Canonical artifacts live under `.grepple/`:

1. `.grepple/project.workspace/overview.mmd`
   - Start here when ownership or repository shape is unknown.
   - Shows modules, packages, entrypoints, routes, and package/import relationships.
2. `.grepple/<package>.package/overview.mmd`
   - Use for a package's public shape and major internal boundaries.
   - Compactly shows types, exported functions, tagged contracts, routes, and grouped relation evidence.
3. `.grepple/<package>.package/structure.mmd`
   - Use when exact fields, methods, underlying types, or relation evidence matter.
   - Exhaustive and therefore more expensive to read.
4. `.grepple/<package>.package/manifest.json`
   - Use only for machine-readable or lossless facts not convenient in Mermaid.
   - Do not outline or read the whole manifest for orientation; large manifests can be thousands of lines.

Generated artifacts are read-only. Never edit `.grepple` files manually.

## Default lookup workflow

### 1. Establish the boundary

Read the workspace overview first if the owning package is not known:

```text
.grepple/project.workspace/overview.mmd
```

Answer only the immediate questions:

- Which module/package owns the feature?
- Is it an entrypoint, shared package, or leaf?
- Which local packages point to it or are pointed to by it?
- Are routes or external module dependencies involved?

If no workspace bundle exists, use Grepple filename/outline discovery from the local search skill rather than recursively reading the tree.

### 2. Read the owning package overview

Open only:

```text
.grepple/<owner>.package/overview.mmd
```

Use its source ranges and relation labels to identify the likely declaration or function. Prefer relation evidence such as `field X`, `method Y parameter 1`, or route handler names over guessing from filenames.

### 3. Jump to source

Once a diagram gives a location, retrieve the declaration directly:

```bash
grepple --at path/to/file.go:40-80
grepple --at path/to/file.ts:25
```

Do not read the whole file unless the declaration itself requires it.

### 4. Expand only when the question requires coupling or behavior

```bash
grepple --related --at path/to/file.go:40
grepple --follow-related 1 --at path/to/file.go:40
grepple --related -F 'Symbol(' path/to/package --limit 8
```

Use:

- `--related` for immediate callees and potential callers.
- `--follow-related 1` when one bounded inline hop replaces several lookups.
- deeper following only when the first hop leaves a specific unanswered question.

Navigation is syntax-based. Treat `[candidate]` edges as leads to verify, not type-checked facts.

## Choose the query by intent

### “Where does this capability live?”

1. Workspace overview.
2. Owning package overview.
3. `--at` the most relevant declaration.

### “What is the public or data model shape?”

1. Package overview.
2. Package structure only if the overview omits needed members.
3. `--at` the exact type range to verify implementation details.

### “What calls this, and what does it call?”

1. Use a diagram location or a bounded symbol search.
2. Run `--related`.
3. Generate a focused flow only when a visual multi-hop path is useful.

### “What will this change affect?”

Inspect all three layers before concluding:

1. Workspace import edges for package-level impact.
2. Package type relations for model/API impact.
3. `--related` on the changed entrypoint and shared helpers for behavioral impact.

### “Why does this route or entrypoint exist?”

1. Find the route/entrypoint in workspace or package overview.
2. Jump to the handler location.
3. Follow related calls one level.

## Generate a focused diagram when canonical views are too broad

Prefer `/tmp` output for exploratory diagrams; do not add ad hoc generated files to the repository.

Focused structure:

```bash
grepple extract structure path/to/model.go \
  --entry Model \
  --source . \
  --output /tmp/model.structure.mmd

grepple extract check structure /tmp/model.structure.mmd .
```

Use a source location when a name is ambiguous:

```bash
grepple extract structure --at path/to/model.ts:25 \
  --source . \
  --output /tmp/model.structure.mmd
```

Focused flow:

```bash
grepple extract flow --at path/to/service.go:80 \
  --source . \
  --depth 2 \
  --max-nodes 30 \
  --output /tmp/service.flow.mmd

grepple extract check flow /tmp/service.flow.mmd .
```

Start with depth 1–2. A large flow is usually less useful than a narrower source root or entrypoint. If generation exceeds `--max-nodes`, first reduce depth or scope; raise the limit only when the additional graph is genuinely needed.

## Validate before trusting canonical artifacts

For architecture-sensitive work, verify the relevant bundle is current:

```bash
grepple extract check workspace .grepple/project.workspace
grepple extract check package .grepple/<package>.package
```

In this repository, the aggregate check is:

```bash
make schema-check
```

If a bundle is stale, regenerate it through the documented Make target or `grepple extract structure`; do not patch generated Mermaid or JSON.

## Avoid these slow paths

- Do not begin with the full package manifest.
- Do not read every package overview “just in case.”
- Do not use broad symbol occurrence searches to answer ownership questions already encoded in the workspace.
- Do not generate a full-depth flow before checking immediate related edges.
- Do not treat a terminal-name navigation candidate as a confirmed call target.
- Do not stop at architecture artifacts when the task depends on runtime behavior, error handling, or side effects.

## Efficient handoff format

When reporting an architectural lookup, preserve only:

- owning module/package;
- relevant declaration locations;
- important package/type/call edges;
- whether each edge is canonical, uniquely resolved, or only a candidate;
- the smallest next source location to inspect.

This keeps later work anchored to evidence without carrying full diagrams or manifests forward.
