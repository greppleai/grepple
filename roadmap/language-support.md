# Language support roadmap

Grepple searches every text file. The items below refer to structural Tree-sitter support: language detection, enclosing-code segments, and symbol outlines.

## Minimum language set

- [x] Go (`.go`)
- [x] JavaScript and JSX (`.js`, `.jsx`)
- [x] TypeScript and TSX (`.ts`, `.tsx`)
- [x] Python (`.py`, `.pyi`, `.pyw`)
- [x] Java (`.java`)
- [x] Kotlin (`.kt`, `.kts`)
- [x] C# (`.cs`)
- [x] C (`.c`, `.h`)
- [x] C++ (`.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`)
- [x] Rust (`.rs`)
- [x] Shell (`.sh`, `.bash`, `.zsh`)

Markdown has heading-aware segments and outlines. JSON and YAML have lightweight structural outlines; their search results currently use plain-text segments.

Dependency note: C# is intentionally pinned to `tree-sitter-c-sharp` `v0.23.4`. The `v0.23.5` `bindings/go` nested module declares a mismatched module path and excludes the parent `src/parser.c` from its module archive. Recheck upstream packaging before upgrading.

## Baseline hardening

- [ ] Add larger golden fixtures for every supported language, including malformed and partially written source.
- [ ] Add language-specific tests for nested declarations, annotations/decorators, generics, and multiline signatures.
- [ ] Decide whether `.h` should remain deterministically classified as C or use repository/content context to distinguish C++ headers.
- [ ] Detect extensionless shell scripts by shebang without misclassifying arbitrary executable text.
- [ ] Add per-language parse and segment benchmarks and record acceptable regression thresholds.
- [ ] Verify grammar upgrades against language parity, outline golden tests, and the parse-once invariant before updating versions.

## Next languages

- [ ] Swift
- [ ] Ruby
- [ ] PHP
- [ ] Scala
- [ ] Terraform/HCL
- [ ] Protocol Buffers
- [ ] SQL, with an explicit dialect policy
- [ ] Dart
- [ ] Lua
- [ ] Elixir
- [ ] Objective-C
- [ ] R
- [ ] Clojure
- [ ] Groovy
- [ ] Perl
- [ ] Zig

## Definition of done for a new Tree-sitter language

- [ ] Pin a grammar with Go bindings compatible with `github.com/tree-sitter/go-tree-sitter`.
- [ ] Add a dedicated `languageAdapter` implementation and register it in `parser/language.go`.
- [ ] Map only extensions covered by tests.
- [ ] Configure structural, context, container, function, block, and name node kinds.
- [ ] Implement useful top-level and nested symbol outlines.
- [ ] Add language detection, outline, segment, malformed-source, and parse-once coverage.
- [ ] Document any ambiguous extensions or unsupported syntax.
