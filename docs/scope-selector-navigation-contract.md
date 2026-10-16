# Scope-selector presentation contract

## Purpose

Human discovery and navigation output may offer a copyable local selector only as
`@KIND:START-END`. It is used as a separate argument:

```sh
grepple --at PATH @KIND:START-END
```

The token means the current local parser has verified that exact inclusive span;
it is not a decoration of a legacy `PATH:LINE[-END]` location. Declaration kinds are stable parser-owned words chosen for the effective source language (for example TypeScript `function`, Python `def`, Rust `fn`, Go `func`), never outline/navigation labels or raw grammar node names. The old normalized `func` spelling remains a local input-only compatibility alias for native function scopes.

## Decision

Add an **opt-in human presentation** shared by local outline and local
navigation destinations. The final flag name and placement will be selected with
CLI compatibility review in implementation, but it must not change default human
output or existing JSON schemas. A later additive JSON field is allowed only when
it carries the same independently verified token; it is not required for the
first implementation.

When enabled, an eligible item presents its normal name and location plus the
verified token (outlines should prefer `Name @kind:start-end`). Do not derive the
token by renaming a displayed kind or copying a stored range. The presentation
boundary must load current local source and ask the parser for the verified
public scope at that range. It must return no token for missing, ambiguous,
stale, malformed, unsupported, plain-text, remote, or artifact-qualified source.
This needs a reusable parser-facing helper that discovers a verified selector for
a candidate range, rather than asking every renderer to loop over kind strings.
It should reuse a caller-owned parsed document where available and be bounded to
one parse per local file/presentation batch.

## Inventory and required fallbacks

| Surface | Existing location | Why it is not itself a selector | Planned treatment |
| --- | --- | --- | --- |
| Human outline | `START-END kind Name` | Outline kinds differ from parser selector names; some have no selector (`type`, `const`); tiny files can render raw content instead. | Verify each symbol against current source; emit the language-native token only when verified. Do not raw-content-fallback in selector presentation. |
| Line-only search annotations | `@kind:start-end` | Typed annotation roles are a remote-capable normalized wire contract. | Preserve their normalized role kinds; accept `@func` as a local function alias. Do not claim they are native local presentation without reparsing source. |
| Search `--related` and related type definitions | `PATH:START-END` | It may name a local declaration, an artifact-qualified dependency, or unresolved evidence. | Append a token only for a current local declaration that independently verifies; retain legacy path/range, `call:LINE`, confidence and provenance. |
| Graph resolve/query/build/diff | declaration `PATH:START-END`; follow-up graph commands use `PATH:START` | Navigation spans may include attached comments and graph data carries no parser verification/source content. For example, Go `Outline` graph location is `10-19`, while only `@func:11-19` verifies. | Resolve a fresh local parser token when source is available; otherwise leave the location and commands unchanged. Never substitute a comment-inclusive range. |
| Directory architecture facts | symbol `PATH:START-END`; relation evidence `PATH:LINE` | Facts can be declarations, while relation evidence can be an import/call line rather than a scope. | Tokenize independently verified local declaration facts only; retain evidence lines as legacy links. |
| Remote search/graph and dependency artifacts | remote path/range or repository-qualified artifact path | `--at PATH @KIND` is explicitly local-only and artifact provenance is not a local source guarantee. | Never emit a copyable local selector. |

## Evidence and negative cases

The current `--at` resolver rejects unknown kinds, malformed or stale ranges,
plain source and ambiguous syntax. Tests must prove emitted tokens round-trip
verbatim through that resolver. Include Go comment-leading declarations,
TypeScript `function`/`type`, nested and one-line scopes, malformed code,
structured same-line ambiguity, raw-content outline fallback, remote/artifact
results and architecture relation evidence. Existing default human and JSON
outputs remain compatibility baselines.