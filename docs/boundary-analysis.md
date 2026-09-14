# Boundary pattern analysis

`grepple boundaries [PATH...]` reports two complementary boundary signals. Workflow analysis treats each resolved target declaration's source file as its owner and retains callable sets, ordered sequences, or member-plus-call combinations repeated across at least two external files. Type-spread analysis reports imported concrete types and unambiguously owned project types used from at least two files. Together they ask whether implementation behavior or a concrete representation has spread beyond its intended boundary.

The analyzer consumes `parser.NavigationGraph`, not grammar node names. Consequently the workflow contract is identical across Go, Java, Kotlin, JavaScript/JSX, TypeScript/TSX, Python, C#, C, C++, Rust, and Shell. Type identity is import-qualified where an adapter can resolve an import; otherwise project-owned types require an unambiguous declaration owner. Unsupported and malformed files retain the existing safe navigation fallback.

## Reported signals

Workflow boundary candidates report:
- **External consumer breadth**: distinct functions, files, and path-derived groups outside the owner.
- **External callable surface**: called owner-file declarations over all callable declarations in that file.
- **Callable co-usage**: the normalized owner callables used by the same external caller.
- **Ordered sequences**: source-ordered calls into the owner file.
- **Member + call combinations**: normalized sets such as `State(read) + Retry(write) + Schedule()` where member ownership can be resolved.

Type boundary candidates report:
- **Canonical identity**: an import-qualified concrete type, or an unambiguously owned project type.
- **Dependency origin**: `local`, `first-party`, `standard-library`, `third-party`, or `unresolved`.
- **Reach**: distinct usages, functions, files, and path-derived groups.
- **Source split**: production files versus recognizable test/benchmark files.
- **Roles**: parameter, result, receiver, local, or unknown usage.
- **Public exposure**: public callable signatures that mention the type.
- **Evidence locations**: bounded human locations and complete JSON usage details.

Resolved target IDs are preferred. Ambiguous calls are included only when every candidate agrees on language, owner file, and terminal callable name; these are counted in `unresolvedCalls` so uncertain evidence remains visible.

Workflow patterns are grouped by caller and filtered by `--min-occurrences` (default 2), then must span at least two distinct external files. Type candidates use the same callable and two-file thresholds. Primitive and unresolved ownerless local types are omitted. Imported types remain visible even when origin is unresolved because concrete dependency reach and public exposure are facts rather than asserted violations. Origin is resolved from local module/package identities and relative imports first; Go standard-library versus third-party imports are deterministic, while `node:`, `java`/`javax`, `kotlin`, and `System` namespaces supply conservative standard-library evidence for their languages. Ambiguous aliases and ecosystems without safe provenance remain `unresolved`. The compatibility `external` JSON field continues to mean only that `importPath` is non-empty. Human output shows 20 candidates per section by default and discloses omissions; `--limit 0` shows every candidate, while JSON is complete under `grepple-boundaries-v2`.

```text
boundary analysis paths=src files=84 workflow-candidates=1 workflow-shown=1 type-candidates=1 type-shown=1
owner: src/request.go [go]
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
  type: github.com/vendor/parser.Node [go, origin=third-party]
    reach: 42 usages / 31 functions / 9 files / 2 packages
    source split: 8 production files / 1 test files
    roles: 28 parameters / 3 results / 0 receivers / 11 locals / 0 unknown
    ! 1 public external-type exposures: src/navigation.go:40 BuildGraph (parameter)
```

## Cache

The command automatically stores its complete, resolved navigation graph under:

```text
.grepple/cache/boundaries/<input-digest>.json
```

The digest covers source paths and bytes, the file limit, language identity, grammar ABI, and generated grammar fingerprint. Any source or grammar change therefore causes a cache miss. Cache writes use an atomic rename, corrupt entries are ignored, and inability to write the optional cache does not fail analysis. `.grepple/cache/` is ignored by Git.

Use `--no-cache` for a forced clean analysis. Cache hits and misses are intentionally absent from human and JSON output, keeping reports byte-for-byte deterministic across cold and warm runs. Source truncation metadata remains explicit when applicable.

## Member and type boundaries

Member evidence records receiver-qualified field/property accesses and classifies writes from assignment/update context. It deliberately does not infer ownership for unqualified identifiers, dynamic computed properties, or unresolved bare field names. Type evidence preserves normalized names, resolved import paths, usage roles, and callable locations. Project-owned type spread is emitted only when method/container declarations establish one unambiguous owner file; imported concrete types do not require a local owner. This keeps C, C++, dynamic-language, and malformed-source behavior deterministic without inventing ownership. Languages without useful type/member syntax, such as Shell, return no fabricated evidence.
