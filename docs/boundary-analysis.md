# Boundary pattern analysis

`grepple boundaries [PATH...]` reports four complementary boundary signals: repeated owner-file workflows, concrete type spread, externally used owned fields/properties, and policy-backed facade bypasses. Findings remain source-linked review leads rather than violations.

The analyzer consumes `parser.NavigationGraph`, not grammar node names. Consequently the workflow contract is identical across Go, Java, Kotlin, JavaScript/JSX, TypeScript/TSX, Python, C#, C, C++, Rust, and Shell. Type identity is import-qualified where an adapter can resolve an import; otherwise project-owned types require an unambiguous declaration owner. Human and JSON reports include discovered, selected, parsed, skipped, failed, and recovered source counts; `--max-files` omissions remain a separate truncation record.

Use `--server URL --repo OWNER/REPO[@REF]` to run complete-source boundary analysis against one exact indexed checkout. The backend loads the selected checkout's `.grepple/boundary-policy.json`, or a repository-relative `--policy` path, and rejects malformed or escaping policy paths. Remote JSON retains repository and commit identity around the nested `grepple-boundaries-v3` report.

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
- **Evidence locations**: bounded human locations and complete JSON usage details.

Resolved target IDs are preferred. Ambiguous calls are included only when every candidate agrees on language, owner file, and terminal callable name; these are counted in `unresolvedCalls` so uncertain evidence remains visible.

Workflow patterns are grouped by caller and filtered by `--min-occurrences` (default 2), then must span at least two distinct external files. Type candidates use the same callable and two-file thresholds. Primitive and unresolved ownerless local types are omitted. Imported types remain visible even when origin is unresolved because concrete dependency reach and public exposure are facts rather than asserted violations. Origin is resolved from graph-level repository roots, local module/package identities, and relative imports first. Go repository roots include every selected file's nearest module plus unambiguous local `go.mod`/`go.work` replacements; conflicting replacements are not asserted. Go then uses an explicit standard-library root set and host-qualified third-party paths; ambiguous unqualified imports remain `unresolved`. The `node:`, `java`/`javax`, `kotlin`, and `System` namespaces provide conservative standard-library evidence for their languages. Ambiguous aliases and ecosystems without safe provenance also remain `unresolved`. The compatibility `external` JSON field continues to mean only that `importPath` is non-empty.

Risk is deliberately conservative. Third-party types in public production signatures or public fields are `critical`; private third-party production spread is `high`; first-party or unresolved public exposure and first-party cross-package spread are `medium`; project-owned public API or cross-package spread and other imported production leads are `low`; standard-library, test-only, and private package-internal local spread are `informational`. Repository-declared utility hubs, test frameworks, declarative configuration, lifecycle cleanup, and adapter protocols lower priority by one level without removing evidence. Workflow repetition is `low` within a package and `medium` across packages. Reasons explain classifications; signals identify reviewed heuristic shapes such as owner cohesion, repeated protocols, parallel abstractions, transitive public exposure, and policy-backed misplaced functions. Human output shows 20 candidates per section by default and discloses omissions; `--limit 0` shows every candidate, while JSON is complete under `grepple-boundaries-v3`.

```text
boundary analysis paths=src files=84 workflow-candidates=1 workflow-shown=1 type-candidates=1 type-shown=1
owner: src/request.go [go, spread=cross-layer, containment=unknown, risk=medium]
  reasons: repeated-owner-file-workflow, cross-package-workflow
  external consumers: 18 functions / 12 files / 5 groups
  external callable surface: 3/5 callables
  repeated callable co-usage (minimum 2 callers in 2+ files):
    Parse + Validate + Normalize
      18 occurrences / 12 files / 5 groups
      callers: src/create.go:20 Create; src/update.go:18 Update; +16 more
  repeated ordered sequences (minimum 2 callers in 2+ files):
    Parse -> Validate -> Normalize
      18 occurrences / 12 files / 5 groups
  repeated member + call combinations (minimum 2 callers in 2+ files):
    Retry(write) + Schedule() + State(read)
      9 occurrences / 7 files / 4 groups

type boundary spread:
  type: github.com/vendor/parser.Node [go, origin=third-party, spread=public-api, containment=unknown, risk=critical]
    reasons: third-party-public-api
    reach: 42 usages / 31 functions / 9 files / 2 packages
    source split: 8 production files / 1 test files
    roles: 28 parameters / 3 results / 0 receivers / 2 fields / 11 locals / 0 unknown
    surfaces: 1 public API / 30 private signatures / 2 field representations / 11 body-local / 0 unknown
    ! 1 public third-party exposures: src/navigation.go:40 BuildGraph (parameter)
```

## Repository-owned policy

By default Grepple uses only generated ownership facts: same-directory consumers can establish `package-internal` and `approved`; cross-boundary containment remains `unknown`. Add `.grepple/boundary-policy.json`, or pass `--policy PATH`, to declare intent:

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

## Cache

The command uses two cache layers: parser-owned path-neutral per-file facts shared with graph, related-navigation, and extraction workflows, plus resolved graph inputs for the selected boundary source universe:

```text
.grepple/cache/navigation/<content-and-grammar-digest>.json
.grepple/cache/boundaries/<input-digest>.json
```

The digest covers source paths and bytes, relevant `go.mod`/`go.work` repository-identity files, the file limit, language identity, grammar ABI, and generated grammar fingerprint. Any source, repository identity, or grammar change therefore causes a cache miss. Cache schema `grepple-boundary-cache-v6` stores graph inputs with repository roots, import targets, and field visibility; policy is applied after cache loading. Cache writes use an atomic rename, corrupt entries are ignored, and inability to write the optional cache does not fail analysis. `.grepple/cache/` is always excluded from graph discovery and ignored by Git.

Use `--no-cache` for a forced clean analysis that bypasses both layers. Cache hits and misses are intentionally absent from human and JSON output, keeping reports byte-for-byte deterministic across cold and warm runs. Source truncation metadata remains explicit when applicable.

## Member and type boundaries

Member evidence records receiver-qualified field/property accesses and classifies writes from assignment/update context. External field/property surface counts only accesses whose receiver type has one unambiguous owner and whose member has an adapter-produced field fact in that owner file. Field visibility is adapter-owned and requires both a public owner and public field. Type evidence preserves normalized names, resolved import paths, usage surface, and locations. Project-owned type spread is emitted only when method/container declarations establish one unambiguous owner file; imported concrete types do not require a local owner. This keeps unsupported or malformed syntax deterministic without inventing ownership.
