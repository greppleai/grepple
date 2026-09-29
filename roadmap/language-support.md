# Language support roadmap

Grepple searches every readable text file. The checked languages below have Tree-sitter-backed segments, outlines, and syntax-based navigation; they do not imply complete type checking or resolution. The generated feature-by-feature matrix and limitations live in [`docs/file-type-support.md`](../docs/file-type-support.md).

## Supported Tree-sitter languages

- [x] Go (`.go`)
- [x] JavaScript and JSX (`.js`, `.jsx`)
- [x] TypeScript and TSX (`.ts`, `.mts`, `.cts`, `.tsx`)
- [x] Python (`.py`, `.pyi`, `.pyw`)
- [x] Java (`.java`)
- [x] Kotlin (`.kt`, `.kts`)
- [x] Dart (`.dart`)
- [x] Swift (`.swift`)
- [x] C# (`.cs`)
- [x] C (`.c`, `.h`)
- [x] C++ (`.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`)
- [x] Rust (`.rs`)
- [x] PHP (`.php`)
- [x] Shell (`.sh`, `.bash`, `.zsh`)

Markdown has heading-aware segments and outlines. JSON and YAML have lightweight structural outlines; their search results use plain-text segments.

## Navigation baseline

All Tree-sitter-backed languages above support `--at`, bounded callees and potential callers through `--related`, and recursive unique-callee expansion through `--follow-related`. TypeScript and TSX share one declaration namespace. Other language boundaries remain isolated. Generic callable declarations and calls are represented by `parser.NavigationGraph`; search consumes that graph directly, and extraction records the same graph from its already parsed trees for semantic enrichment and flow-parity checks. Resolution remains syntax-based: for example, Swift resolves same-file calls but does not infer SwiftPM/Xcode module ownership.

Go additionally recognizes interface methods and function-valued struct fields. Navigation remains syntax-based; package/import, receiver, and declaration-kind context is used when it can narrow candidates safely, while type-checked dispatch, inheritance-aware overrides, overload selection, dynamic calls, and potential cross-language FFI edges remain future hardening work.

## Baseline hardening

- [ ] Add larger golden fixtures for every supported language, including malformed and partially written source.
- [ ] Add language-specific tests for nested declarations, annotations/decorators, generics, and multiline signatures.
- [ ] Decide whether `.h` should remain deterministically classified as C or use repository/content context to distinguish C++ headers.
- [ ] Detect extensionless shell scripts by shebang without misclassifying arbitrary executable text.
- [ ] Add per-language parse and segment benchmarks and record acceptable regression thresholds.
- [ ] Verify grammar upgrades against language parity, outline golden tests, and the parse-once invariant before updating versions.

## Cross-feature parity

Language identity, extensions, grammar fingerprints, and parser/navigation capabilities remain canonical in `parser`; extraction and GritQL retain their feature-specific adapters. The current cross-feature matrix and active implementation routes are documented in [`docs/file-type-support.md`](../docs/file-type-support.md). Expose the same facts through one deterministic capability view without creating a second language registry.

- [x] Add human-readable, JSON, and generated-Markdown capability output covering segments, outlines, navigation, focused structure/flow, native GritQL, directory architecture, adapter-owned import relations, and normalized declaration/call/import/type/field/member/entrypoint facts with unsupported distinguished from unresolved evidence.
- [x] Add JavaScript/JSX to native `gritql-v1` by reusing TypeScript-family behavior where the grammars agree and adding dedicated JavaScript/JSX conformance fixtures.
- [x] Add JavaScript/JSX focused structure and flow extraction over the shared navigation graph and ECMAScript helpers.
- [x] Add parity tests that fail when advertised support lacks implementation or unsupported combinations silently fall back.
- [x] Add Python to native `gritql-v1` with dedicated query contexts, dotted-import placeholders, conformance fixtures, scanner/CLI coverage, and explicit malformed-source behavior.
- [x] Add C, C++, C#, Java, Kotlin, Rust, and Shell to native `gritql-v1`; all parser-backed Tree-sitter languages now have registered adapters and parity coverage.
- [x] Add Python focused structure/flow with class inheritance, annotated and unannotated attributes, decorators, `.pyi` stubs, and shared navigation-graph calls.
- [x] Add adapter-owned module-level Python `import` and `from ... import ...` facts, alias-aware call context, absolute and explicit-relative local module resolution, `.py`/`.pyi`/`.pyw` targets, ambiguity preservation, architecture relations, and capability reporting while leaving function-local imports unsupported.
- [x] Add adapter-owned Java package/type/wildcard/static imports and Kotlin package/declaration/wildcard/aliased imports, resolving local targets through syntax-evidenced package and top-level export facts for graph calls and directory architecture.
- [x] Add adapter-owned C# file-scoped and single-block namespace facts plus compilation-unit and namespace-local `using`, alias, static-owner, and global-using facts, resolving public top-level targets while leaving multi-namespace files unscoped.
- [x] Add parser-owned Rust external and inline module identities, grouped/aliased/glob `use`, `crate`/`self`/`super`, visibility-aware re-exports, direct normal and raw string-literal `#[path]` targets, selected crate-root traversal, ambiguity preservation, scoped call resolution, directory architecture, nested focused ownership, visibility-checked unambiguous same-crate cross-file `impl` attachment, and conservative private/`pub(crate)`/`pub(super)`/`pub(self)`/`pub(in path)` module, import, and call resolution while leaving external crates and conditional path ownership unresolved.
- [x] Add Java focused structure/flow for classes, interfaces, records, enums, inheritance, fields, constructors, methods, and graph-backed calls.
- [x] Add Kotlin focused structure/flow for classes, interfaces, objects, data-class constructor properties, delegation inheritance, properties, functions, and graph-backed calls.
- [x] Add C# focused structure/flow for classes, interfaces, structs, records, enums, inheritance, properties, fields, constructors, methods, visibility, static members, project-root discovery, and graph-backed calls.
- [x] Add conservative Rust focused structure/flow for structs, tuple structs, enums, traits, same-file impl blocks, fields, variants, associated types, methods, visibility, trait implementations, Cargo-root discovery, and graph-backed calls without framework semantics.
- [x] Define and implement conservative C/C++ focused architecture contracts without inferring preprocessor, template, linkage, ABI, or build-system semantics.

## Recent language addition

- [x] Terraform/HCL (syntax-backed segments and outlines, literal top-level block declarations, function-call facts, and read-only GritQL; reference resolution remains planned below)

Later languages remain demand-driven rather than implicitly ordered:

- [ ] Ruby
- [ ] Scala
- [ ] Protocol Buffers
- [ ] SQL, with an explicit dialect policy
- [ ] Lua
- [ ] Elixir
- [ ] Objective-C
- [ ] R
- [ ] Clojure
- [ ] Groovy
- [ ] Perl
- [ ] Zig

## Terraform/HCL reference resolution (planned)

HCL navigation currently records literal top-level declarations and function invocations, but traversals such as `aws_instance.web.id`, `var.ami`, `local.name`, and `module.app.output` do not link to declarations. Treat traversals as references, not function calls; do not infer Terraform evaluation semantics.

- [ ] Index literal resource, data, variable, locals, and module declarations across `.tf` files in the same Terraform directory, retaining source locations and preserving duplicate or ambiguous addresses.
- [ ] Resolve unambiguous static resource/data addresses, `var.<name>`, `local.<name>`, and the `module.<name>` declaration prefix to their local definitions. Leave computed/dynamic traversals and `module.<name>.<output>` target resolution explicitly unresolved rather than guessing.
- [ ] Add cross-file fixtures and navigation tests for resolved, missing, ambiguous, and unsupported references, including source ranges and separation of references from HCL function calls; document capability limits.
- [ ] Investigate module-source/output, provider and alias, `count`/`for_each`, and indexed/dynamic-address resolution as separate, evidence-backed follow-ups. Preserve unresolved status when module boundaries, workspace context, or Terraform evaluation are required.

## Definition of done for a new Tree-sitter language

- [ ] Pin a grammar with Go bindings compatible with `github.com/tree-sitter/go-tree-sitter`.
- [ ] Add a dedicated `languageAdapter` implementation and register it in `parser/language.go`.
- [ ] Map only extensions covered by tests.
- [ ] Configure structural, context, container, function, block, and name node kinds.
- [ ] Implement useful top-level and nested symbol outlines.
- [ ] Configure callable declarations and call-expression extraction for `--at` and related navigation.
- [ ] Add language detection, outline, segment, malformed-source, and parse-once coverage.
- [ ] Document any ambiguous extensions or unsupported syntax.
