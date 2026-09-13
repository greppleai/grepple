# Responsibility analysis

`grepple responsibilities TYPE [PATH...]` finds repeated interaction topology around a type without comparing source text.

The analyzer consumes `parser.NavigationGraph`, not grammar node names. Consequently the analysis and output contract are identical across Go, Java, Kotlin, JavaScript/JSX, TypeScript/TSX, Python, C#, C, C++, Rust, and Shell. Languages without receiver/container declarations naturally produce no type report; unsupported and malformed files retain the existing safe navigation fallback.

## Reported signals

- **Type usage breadth**: distinct external functions, files, and packages with explicit type bindings or interactions.
- **External method surface**: distinct externally called methods over distinct declared methods.
- **Method co-usage**: the sorted set of methods used by the same caller.
- **Ordered sequences**: the source-ordered type-method sequence within each caller.
- **Member + method combinations**: normalized sets such as `State(read) + Retry(write) + Schedule()` used by the same caller.

Methods invoked from another method on the analyzed type are excluded from external consumer counts. Resolved target IDs are preferred. Candidate target IDs and normalized receiver types provide deterministic fallback evidence and are counted in `unresolvedCalls` so uncertain matches remain visible.

Patterns are grouped by caller, ordered by occurrence count and then normalized signature, and filtered by `--min-occurrences` (default 2).

```text
Request
  languages: go
  consumers: 18 functions / 12 files / 5 packages
  external method surface: 3/5 methods
  analyzed files: 84

Repeated method co-usage (minimum 2 callers):
  Normalize + Parse + Validate
    18 occurrences / 12 files / 5 packages

Repeated ordered method sequences (minimum 2 callers):
  Parse -> Validate -> Normalize
    18 occurrences / 12 files / 5 packages

Repeated member + method combinations (minimum 2 callers):
  Retry(write) + Schedule() + State(read)
    9 occurrences / 7 files / 4 packages
```

## Cache

The command automatically stores its complete, resolved navigation graph under:

```text
.grepple/cache/responsibilities/<input-digest>.json
```

The digest covers source paths and bytes, the file limit, language identity, grammar ABI, and generated grammar fingerprint. Any source or grammar change therefore causes a cache miss. Cache writes use an atomic rename, corrupt entries are ignored, and inability to write the optional cache does not fail analysis. `.grepple/cache/` is ignored by Git.

Use `--no-cache` for a forced clean analysis. Cache hits and misses are intentionally absent from human and JSON output, keeping reports byte-for-byte deterministic across cold and warm runs. Source truncation metadata remains explicit when applicable.

## Member-access boundary

Version 1 records receiver-qualified field/property accesses and classifies writes from assignment/update context. It deliberately does not infer ownership for unqualified identifiers, dynamic computed properties, or unresolved bare field names. This keeps C, C++, dynamic-language, and malformed-source behavior deterministic without inventing type ownership. Languages without type/member syntax, such as Shell, use the same analyzer and return no fabricated type evidence.
