# Boundary pattern analysis

The internal boundary analyzer (and remote analysis API) reports repeated owner-file workflows, concrete type spread, externally used owned fields/properties, and policy-backed facade bypasses. The standalone `grepple boundaries` command was removed; these findings are source-linked review leads, not violations.

The analyzer consumes source-backed navigation facts, not grammar node names. The backend accepts an `api.AnalysisRequest` with `Operation: api.AnalysisBoundaries` and one indexed `Repository`. Set `MinOccurrences` to a positive number (typically 2), and optionally set `Paths`, `MaxFiles`, and `Policy`. The backend loads the selected checkout's `.grepple/boundary-policy.json` by default or a repository-relative policy path, rejecting malformed or escaping paths. The JSON response includes repository and commit identity, completeness, source counts, and the nested `grepple-boundaries-v3` report. There is no standalone boundaries CLI command.

## Reported signals

Workflow boundary candidates report:
- **Risk and reasons**: a low/medium workflow review priority with stable evidence reasons; this remains a heuristic lead.
- **External consumer breadth**: distinct functions, files, and path-derived groups outside the owner.
- **External callable surface**: called owner-file declarations over all callable declarations in that file.
- **Callable co-usage**: the normalized owner callables used by the same external caller.
- **Ordered sequences**: source-ordered calls into the owner file.
- **Member + call combinations**: normalized sets such as `State(read) + Retry(write) + Schedule()` where member ownership can be resolved.

Type boundary candidates report:
- **Canonical identity**: an import-qualified concrete type, or an unambiguously owned project type.
- **Dependency origin**: `local`, `first-party`, `standard-library`, `third-party`, or `unresolved`.
- **Risk and reasons**: `critical`, `high`, `medium`, `low`, or `informational` plus stable reason identifiers.
- **Reach**: distinct usages, functions, files, and path-derived groups.
- **Source split**: production files versus recognizable test/benchmark files.
- **Roles**: parameter, result, receiver, local, or unknown usage.
- **Surfaces**: public API, private signature, field representation, body-local, or unknown usage.
- **Spread and containment**: `package-internal`, `cross-package`, `cross-layer`, or `public-api` reach plus `approved`, `escaped`, or `unknown` containment.
- **Public exposure**: public callable signatures and public field representations that mention the type.
- **Evidence locations**: source-linked locations and complete JSON usage details.

Resolved target IDs are preferred. Ambiguous calls are included only when every candidate agrees on language, owner file, and terminal callable name; these are counted in `unresolvedCalls` so uncertain evidence remains visible.

Workflow patterns are grouped by caller and filtered by the request's positive `MinOccurrences` (typically 2), then must span at least two distinct external files. Type candidates use the same callable and two-file thresholds. Primitive and unresolved ownerless local types are omitted. Imported types remain visible even when origin is unresolved because concrete dependency reach and public exposure are facts rather than asserted violations. Origin is resolved from graph-level repository roots, local module/package identities, and relative imports first. Go repository roots include every selected file's nearest module plus unambiguous local `go.mod`/`go.work` replacements; conflicting replacements are not asserted. Go then uses an explicit standard-library root set and host-qualified third-party paths; ambiguous unqualified imports remain `unresolved`. The `node:`, `java`/`javax`, `kotlin`, and `System` namespaces provide conservative standard-library evidence for their languages. Ambiguous aliases and ecosystems without safe provenance also remain `unresolved`. The compatibility `external` JSON field continues to mean only that `importPath` is non-empty.

Risk is deliberately conservative. Third-party types in public production signatures or public fields are `critical`; private third-party production spread is `high`; first-party or unresolved public exposure and first-party cross-package spread are `medium`; project-owned public API or cross-package spread and other imported production leads are `low`; standard-library, test-only, and private package-internal local spread are `informational`. Repository-declared utility hubs, test frameworks, declarative configuration, lifecycle cleanup, and adapter protocols lower priority by one level without removing evidence. Workflow repetition is `low` within a package and `medium` across packages. Reasons explain classifications; signals identify reviewed heuristic shapes such as owner cohesion, repeated protocols, parallel abstractions, transitive public exposure, and policy-backed misplaced functions. JSON retains the full candidate set under `grepple-boundaries-v3`, with truncation and source diagnostics reported separately.

## Repository-owned policy

By default Grepple uses only generated ownership facts: same-directory consumers can establish `package-internal` and `approved`; cross-boundary containment remains `unknown`. Add `.grepple/boundary-policy.json`, or set the analysis request's `Policy` field to a repository-relative path, to declare intent:

```json
{
  "schema": "grepple-boundary-policy-v1",
  "layers": [
    {"name": "api", "paths": ["api"]},
    {"name": "storage", "paths": ["internal/storage"]}
  ],
  "containments": [
    {"name": "storage-internal", "ownerPaths": ["internal/storage"], "consumerPaths": ["internal/storage", "storage"]}
  ],
  "facades": [
    {"name": "storage", "facadePaths": ["storage"], "implementationPaths": ["internal/storage"]}
  ],
  "classifications": [
    {"category": "adapter-protocol", "paths": ["internal/adapters"]}
  ]
}
```

Paths are repository-relative prefixes or slash-separated glob patterns. Supported intentional classifications are `utility-hub`, `test-framework`, `declarative-configuration`, `lifecycle-cleanup`, and `adapter-protocol`. A facade bypass requires a resolved direct call from outside both facade and implementation path sets; candidate calls are not promoted to bypass findings. Policy is validated before source discovery.

## Member and type boundaries

Member evidence records receiver-qualified field/property accesses and classifies writes from assignment/update context. External field/property surface counts only accesses whose receiver type has one unambiguous owner and whose member has an adapter-produced field fact in that owner file. Field visibility is adapter-owned and requires both a public owner and public field. Type evidence preserves normalized names, resolved import paths, usage surface, and locations. Project-owned type spread is emitted only when method/container declarations establish one unambiguous owner file; imported concrete types do not require a local owner. This keeps unsupported or malformed syntax deterministic without inventing ownership.
