# Responsibility analysis

`grepple responsibilities [PATH...]` detects likely file-boundary leaks. Every resolved target declaration establishes an owner file; calls from other files are grouped by that owner. Only callable sets, ordered sequences, or member-plus-call combinations repeated across at least two external files survive. This asks whether an implementation workflow that belongs behind file X has spread into multiple consumers. Directory scope avoids package/module assumptions and applies equally to class-oriented and function-oriented languages.

The analyzer consumes `parser.NavigationGraph`, not grammar node names. Consequently the contract is identical across Go, Java, Kotlin, JavaScript/JSX, TypeScript/TSX, Python, C#, C, C++, Rust, and Shell. Unsupported and malformed files retain the existing safe navigation fallback. `--type TYPE` selects the separate, broader type-centric report when needed.

## Reported signals

Directory boundary candidates report:
- **External consumer breadth**: distinct functions, files, and path-derived groups outside the owner.
- **External callable surface**: called owner-file declarations over all callable declarations in that file.
- **Callable co-usage**: the normalized owner callables used by the same external caller.
- **Ordered sequences**: source-ordered calls into the owner file.
- **Member + call combinations**: normalized sets such as `State(read) + Retry(write) + Schedule()` where member ownership can be resolved.

Resolved target IDs are preferred. Ambiguous calls are included only when every candidate agrees on language, owner file, and terminal callable name; these are counted in `unresolvedCalls` so uncertain evidence remains visible.

Patterns are grouped by caller and filtered by `--min-occurrences` (default 2), then must span at least two distinct external files. Candidates are ranked by cross-file pattern breadth. General symbol usage and one-off API calls are omitted. Human output shows 20 candidates and up to five patterns per category by default, with explicit omissions and up to three caller locations per pattern; `--limit 0` shows every candidate, while JSON is complete.

```text
responsibility boundaries paths=src files=84 candidates=1 shown=1
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
