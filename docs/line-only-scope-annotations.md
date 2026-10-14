# Typed line-only scope annotations

Status: parser, selected-file search, transport and human/JSON rendering implemented and locally tested. Local `--line-only` content search shows scopes by default; `--no-annotate-scopes` restores plain rows. An annotation describes syntax containing a hit, never an additional text-search hit. Current remote backends without capability acknowledgement reject default annotated line-only searches explicitly; use `--no-annotate-scopes` until backend support arrives. Benchmark overhead varies with file size and worker count; other search modes remain unchanged.

## Source owners

- `internal/cli/search/args.go:25-64,95-147,167-172,215-265` owns flags, validation, default anchors and `--enclosing`. Anchors are disabled for remote/JSON and currently for `--enclosing`.
- `internal/parser/match_range.go:25-59,75-171` parses once per call to find a construct beginning on the hit or the nearest containing multiline named node. It does not classify kinds; `internal/parser/view.go:80-116` exposes node kind/error state; `internal/parser/language.go:79-89,113-148` owns adapter rules.
- `internal/search/search_engine.go:209-233,550-562` projects *windowed* matched files, choosing starting or enclosing ranges. `internal/search/result.go:14-41` sets the existing `ResultMatch.StartLine/EndLine`. Do not redefine those fields.
- `internal/render/renderer_matches.go` preserves plain and anchored rows and appends default line-only scope suffixes; `renderer_scope_annotations.go` validates kinds/bounds from local or capable remote peers; `renderer_json.go` exposes optional typed fields. `render.go` caps human output only.
- `internal/search/{types,model,request,search_params}.go`, `internal/wire/model.go:10-62`, `internal/cli/search/remote.go:20-46,73-76` own the shared local/remote request/results. Old servers may ignore a new request property and omit new result fields.


In eligible line-only searches, `parser.ScopeContentLanguageFor` classifies `.json`, `.yaml` and `.yml` as structured content. This is deliberately separate from legacy `Parser.LanguageFor`, which still classifies those paths as `text`; plain search metadata/output is unchanged. Terraform `.tf.json` remains text in both modes. Stdin without a path, invalid structured data, unsupported syntax and plain text never invent roles.

## CLI and semantics

`--line-only` enables annotations by default for content search. `--no-annotate-scopes` explicitly disables them. Existing `--enclosing`, `--at`, outline, file/count modes, `-o`, and positive context retain their own behavior without annotations. Full/default structural and unrelated JSON output are unchanged. `--line-only --json` and `--json-matches` include typed fields when a span is verified. Project only compact, windowed matches; no parse per hit or extra lines. Use one-based inclusive, source-bounded spans. One-line ranges are valid only for actual classified constructs. No brace/indentation guessing.

Per match, select (1) the smallest meaningful *classified* syntax construct containing the line (`nearest`), excluding transparent wrappers/root/tokens, and (2) the nearest containing navigable callable, else type or content container (`scope`). A method beats its class; an inner function beats its outer function. If no callable, a class/interface/struct/section/config block can be the scope. Choose smallest containing candidate, then deepest node, then stable source order for ties. If the same construct fills both roles, render it once. Omit untrusted or unavailable spans individually; never label an error-recovery subtree by guesswork. An unaffected reliable ancestor is eligible only if its own bounds and kind are proven. Outside comments have no incidental owner. Maximum two distinct spans per hit, nearest then scope.

A versioned **fixed allowlist**, never a source name or raw tree-sitter kind: code `func`, `method`, `class`, `interface`, `struct`, `enum`, `if`, `while`, `for`, `switch`, `case`, `try`, `catch`, `block`, `rule`, `property`, `snippet`; content `section`, `object`, `array`, `mapping`, `sequence`, `document`. Map grammar forms in adapter-owned rules with fixture tests. A generic `block` only denotes a verified meaningful block (e.g. HCL), not an arbitrary wrapper. JS arrow and Go function => `func`; method/constructor => `method`; CSS rule set => `rule` not `func`; Svelte template if => `if`. Omit unknown or ambiguous kinds; do not use arbitrary node names as labels.

## Output and literal source boundary

Illustrative human row:

```text
Af3│27│        return runtime.Start(ctx, name) @case:26-27 @func:15-31
```

Use exactly one `@` per `@KIND:START-END` token, space-separated, at most two; omit each unavailable span. The matched line alone appears; a containing function's declaration does **not** become a match. `@func:START-END` alone appears when nearest and scope are the same declaration. Selector-shaped text is for display only until a separate verified `--at` selector is implemented; today `--at PATH:START-END` retrieves clean source.

Human suffixes are **not self-describing**: source can itself end in ` @func:15-31`, and no length marker separates literal source from appended roles. Do not parse human suffixes as a machine contract or treat decorated hashlines as edit authority. For exact source and typed, unambiguous roles, use `--line-only --json-matches` (the `text`, `nearest`, and `scope` fields) or reread a clean `--at` range. Plain/unclassified lines receive no suffix; preserve their Unicode, pipes and trailing whitespace. Source content containing a lookalike label cannot turn into a parser-verified annotation in JSON.

Even with retained original hash tokens, decorated rows are **not** edit-ready source: reread clean lines with `grepple --line-only --at PATH:START-END` before `grepple write`. `--no-annotate-scopes` restores the old literal and byte-faithful `HASH│LINE│source` output. Keep existing non-anchored location conventions; an output cap must not produce a partial row interpreted as complete. JSON stays uncapped. Full JSON adds optional `nearest` and `scope` objects (`kind`, `startLine`, `endLine`) to `ResultMatch`; compact `--json-matches` adds the same to each flattened match. `text` is unmodified original source; omit unavailable fields. Both JSON role fields may hold the same typed span even when human suffix deduplicates it. Never parse human annotations back into JSON. Preserve legacy `startLine`/`endLine` and their serialization. Matched text containing an embedded line break (possible from a malformed remote response) is rejected before annotated human output instead of creating an ambiguous multi-line row; JSON retains literal text.

## Remote compatibility

The request bit and response-level `scopeAnnotationsVersion: 1` acknowledgement are required for *every* successful annotated response, including empty/no-parseable-scope pages. Missing acknowledgement means an old/unsupported backend, not absence of syntax. Default remote line-only requests now need the capability; `--no-annotate-scopes` restores legacy remote behavior. Fail explicitly before rendering/merging an annotated remote response without acknowledgement; leave local-only operation available. The router must acknowledge only if every contributing shard actually supports it; cursor/page DTOs need equivalent per-page acknowledgement. Do not reparse partial remote match text or silently emit mixed quality annotations. Older clients do not request the feature and ignore additive response fields; old servers must not be assumed to honor unknown JSON fields. Verify old/new router/shard responses in tests; no deployment, auth or trust changes here.

Client implementation (step 0004): request/typed match fields and capability marker are additive in shared `internal/search`, `internal/wire`, and cursor-page DTOs. A remote response without version **1** is rejected *before merging or rendering*, even for empty results, while `--no-annotate-scopes` clients retain legacy behavior. Current `grepple-backend` does not emit this acknowledgement or produce these annotations, so default remote line-only requests will explicitly fail until its router and every serving shard are upgraded together. Mocked version-1 responses test the future protocol; their existence is **not** evidence of backend readiness. Do not set the marker on the router merely because it understands the new request field; wait for verified end-to-end shard support (including cursor pages). Local-only annotation remains usable.

## Acceptance matrix (test targets, not unverified claims)

The live registry is `internal/parser/language.go:122-196`: 18 code adapters, Markdown, JSON, YAML and text. A fixture for a declaration hit and an interior hit (where syntax permits), exact expected kinds/bounds and an honest missing-kind case must be reviewed per ID; derive matrix-completeness checks from the registry. `internal/parser/match_range_test.go:5-100` covers only some languages and untyped bounds today.

| ID | Candidate nearest -> scope test | Owner |
| --- | --- | --- |
| go | `if` within function -> `if`, `func`; method -> `method` | `language_go.go` |
| java | `if` within class method -> `if`, `method`; class field -> `class` | `language_java.go` |
| kotlin | `while` in `fun` -> `while`, `func` | `language_kotlin.go` |
| dart | `if` in class method -> `if`, `method` | `language_dart.go` |
| swift | `for` in function -> `for`, `func` | `language_swift.go` |
| hcl | nested block -> `block`, `block` if distinct; never a fake callable | `language_hcl.go` |
| svelte | template `{#if}` -> `if`, `snippet` only if contained; `<script>` function only if injected AST proves bounds | `language_svelte.go`, `language_svelte_injection.go` |
| css | property in rule -> `property`, `rule`; no callable | `language_css.go` |
| javascript | `if` in arrow/function -> `if`, `func`; class method -> `method` | `language_javascript.go`, `language_ecmascript.go` |
| typescript | `if` in method -> `if`, `method`; type-only interface -> `interface` | `language_typescript.go` |
| tsx | function/JSX -> verified inner kind if available, `func` | `language_typescript.go` |
| python | `if` in `def` -> `if`, `func`; class method -> `method` | `language_python.go` |
| csharp | `while` in method -> `while`, `method`; class -> `class` | `language_csharp.go` |
| c | `if` in function -> `if`, `func`; outside function omit scope | `language_c.go`, `language_c_family.go` |
| cpp | `for` in method -> `for`, `method`; class -> `class` | `language_cpp.go`, `language_c_family.go` |
| rust | `if` in `fn` -> `if`, `func`; `impl` method -> `method` if proven | `language_rust.go` |
| php | `if` in function/method -> `if`, `func`/`method` | `language_php.go` |
| shell | `if` in shell function -> `if`, `func`; top-level `while` only -> `while` | `language_shell.go` |
| markdown | heading section hit -> `section`; before first heading -> none | `language_markdown.go`, `markdown.go:79-90` |
| json | nested object/array -> `object`/`array`, ancestor container where distinct | `structured.go:27-57,73-110` |
| yaml | nested mapping/sequence/document -> `mapping`/`sequence`/`document` if exact bounds proven | `structured.go:27-57,99-153` |
| text | literal matches only, no structural kind/range | `content.go:6-25` |

`structured.go:144-153` computes end lines from descendant start lines, **not** delimiter positions; do not claim a closing-brace line without proof. Markdown ends at the line before the next equal/higher heading (`markdown.go:79-90`). Comments/empty lines, malformed syntax, multi-document YAML and fake suffixes need dedicated cases. If any candidate fails grammar-backed verification, document omission and test that fallback rather than inventing a plausible range.

The specialized content projection (`internal/parser/scope_annotations_{content,markdown,structured}.go`) follows these narrower rules:

- Markdown parses once with its registered block grammar, uses the same top-level ATX/Setext heading recognition and `markdownHeadingEnds` as outline/segments, and gives both roles the deepest containing `section`. Text before the first heading has neither role. A heading nested inside code/list/quote is not a section. Heading titles are never annotation labels.
- JSON must first be valid JSON (not merely YAML-parseable); yaml.v3 supplies its container tree once, and a single delimiter scan ignoring quoted strings supplies exact closing-brace/bracket lines. An `object` or `array` is eligible only when its yaml.v3 start line/column matches a verified opening delimiter; the nearest eligible container and its next containing container form the two roles. A standalone scalar document can have `document`; JSON/YAML keys, values, aliases and source identifiers are never suffix labels.
- YAML block `mapping`/`sequence` bounds end at the last parser-verified *single-line payload node*, not at a guessed closing marker or trailing blank/comment. `document` is available for explicit multi-document streams or a scalar root, bounded to its decoded root payload. Flow collections and multiline/literal/folded scalar endings have no proved end from yaml.v3 starts and are omitted along with any ancestor depending on that end; independently bounded sibling containers may remain. Comments, whitespace-only lines and YAML document separators receive no annotation. Decode the entire YAML stream before emitting any roles, so a malformed later document invalidates the projection for the file. JSON's closing delimiter lines are included; YAML trailing comments/blanks are not.
- Multiple disjoint same-depth containers on the *same hit line* cannot be distinguished without a match column: omit that line's roles. Where only one proven container exists, nearest and scope refer to the same typed span; the human renderer will deduplicate it. Plain `text`, unsupported/binary/empty input and failed parses yield no annotations, never extra match lines. The content projection is additive and does not change `--outline`, `--enclosing`, or explicitly unannotated output.

## Implementation gates

Test each ID, nested/declaration/body and missing bounds; no extra match rows; one-/two-label and same-node cases; literal fake suffix, CRLF/trailing whitespace, both JSON formats, old wire versions, clean `--at` editing, result-window parse counts, output cap and byte-identical `--no-annotate-scopes` golden output. Benchmark disabled vs enabled at 1/100/1000 hits and across file sizes/workers; no need to pay annotation cost outside default line-only searches.
