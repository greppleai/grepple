# `gritql-v1` compatibility contract

`gritql-v1` is Grepple's unified, closed, read-only detection contract for every Tree-sitter-backed language in the capability matrix: `go`, `javascript`, `typescript`, `tsx`, `python`, `java`, `kotlin`, `csharp`, `c`, `cpp`, `rust`, and `shell`. Target syntax is supplied by language adapters while query algebra, transactions, limits, ordering, and diagnostics remain shared.

The contract is not an alias for an upstream GritQL release. A conforming implementation accepts exactly the documented syntax and rejects every other construct; it does not invoke an external engine, Node, a shell, or a fallback interpreter.

## 1. Closed syntax

The grammar is EBNF. Literal words and punctuation are quoted. `EOF` means the end of the pattern body.

```ebnf
pattern          = spacing, language, line_end, spacing, query, spacing, EOF ;
language         = "language", hspace1, ( "c" | "cpp" | "csharp" | "go" | "java" | "javascript" | "kotlin" | "python" | "rust" | "shell" | "typescript" | "tsx" ) ;

query            = prefix, [ spacing, where_clause ] ;
prefix           = snippet
                 | node_pattern, [ req_sep, "as", req_sep, named_metavariable ]
                 | regex
                 | "and", req_sep, query_block
                 | "or", req_sep, query_block
                 | unary, req_sep, prefix
                 | "parent", req_sep, "kind", spacing, "(", spacing, "\"function\"", spacing, ")" ;
unary            = "not" | "maybe" | "contains" | "within" ;

query_block      = "{", spacing, query, spacing, ",", spacing,
                   query, { spacing, ",", spacing, query },
                   [ spacing, "," ], spacing, "}" ;
where_clause     = "where", req_sep, constraint_block ;
constraint_block = "{", spacing, constraint,
                   { spacing, ",", spacing, constraint },
                   [ spacing, "," ], spacing, "}" ;
constraint       = metavariable, spacing, "<:", spacing, ( "empty" | query ) ;
node_pattern     = node_name, "(", [ node_arg, { ",", node_arg } ], ")" ;
node_arg         = metavariable | node_pattern
                 | node_name, "=", ( metavariable | node_pattern ) ;
node_name        = ( "A"…"Z" | "a"…"z" | "_" ),
                   { "A"…"Z" | "a"…"z" | "0"…"9" | "_" } ;
snippet          = "`", { snippet_char | snippet_escape | snippet_newline }, "`" ;
snippet_escape   = "\\`" | "\\\\" ;
snippet_newline  = "\n" | "\r\n" ;
snippet_char     = ? any Unicode scalar value except "`", "\\", CR, or LF ? ;
regex            = "r\"", { regex_char | regex_escape }, "\"" ;
regex_escape     = "\\\"" | "\\\\" | "\\n" | "\\r" | "\\t"
                 | "\\x", hex, hex | "\\u", hex, hex, hex, hex ;
regex_char       = ? any Unicode scalar value except '"', "\\", CR, or LF ? ;
hex              = "0"…"9" | "a"…"f" | "A"…"F" ;

metavariable     = "$_" | "$", meta_start, { meta_continue } ;
named_metavariable = "$", meta_start, { meta_continue } ;
meta_start       = "A"…"Z" | "a"…"z" ;
meta_continue    = meta_start | "0"…"9" | "_" ;

spacing          = { whitespace | comment } ;
req_sep          = whitespace, spacing | comment, spacing ;
hspace1          = ( " " | "\t" ), { " " | "\t" } ;
line_end         = { " " | "\t" }, [ line_comment ], ( "\n" | "\r\n" ) ;
whitespace       = " " | "\t" | "\n" | "\r\n" ;
comment          = line_comment, ( "\n" | "\r\n" | EOF ) ;
line_comment     = "//", { ? any scalar except CR or LF ? } ;
```

The `EOF` alternative in `comment` is lexical only; it does not let `line_end` omit its newline. A pattern must therefore start with exactly one supported `language` directive on its own logical line. Keywords and language identifiers are lowercase and case-sensitive. Comments have no meaning inside snippets or regex literals. Bare CR, invalid UTF-8, empty `and`/`or` blocks, one-element `and`/`or` blocks, and omitted commas are syntax errors.

There are no infix query operators, parentheses, implicit sequences, or precedence rules. The one postfix capture, `node_pattern as $name`, binds the exact matched node before an optional `where`. It does not accept `$_` or arbitrary snippets. Unary prefix operators associate right-to-left, so `not maybe` followed by a snippet means `not (maybe snippet)`. The optional postfix `where` applies to the complete `prefix` immediately before it; to constrain a unary operand instead, place that operand in an `and` or `or` block. A second `where` is invalid. Blocks are comma-separated, evaluated left to right, and may have one trailing comma. These rules make every parse unique.

Valid examples are:

```grit
language go
`exec.Command($args)`
```

```grit
language go
and {
  `exec.Command($args)`,
  not `exec.CommandContext($args)`,
} where {
  $args <: r"^ctx,",
}
```

```grit
language go
contains `http.Client{$_}`
```

```grit
language go
and {
  `{}`,
  not parent kind("function"),
}
```

```grit
language go
type_spec(name=$name, type=struct_type())
```

`type_identifier() as $name` captures the matched identifier node itself rather than one of its children. This is useful for leaf nodes with no named fields, and `where { $name <: r"^[A-Z]" }` can then constrain its exact source-written spelling. The capture preserves structural equality for repeated metavariables, normal source ranges, and the evaluator's resource bounds. Only node patterns support `as` in this v1 subset.

A node pattern matches a pinned Tree-sitter grammar node by its exact kind. Named arguments select direct grammar fields; positional arguments select direct named children in order, ignoring comments and extra nodes. Nested node patterns constrain field/child kinds, and `$name` binds one structural node. Unmentioned fields/children are unconstrained, so `struct_type()` matches any struct body. Node and field names are ASCII and at most 128 bytes; unknown kinds and fields are compile errors, not clean searches. These patterns and bindings work in all twelve supported target languages; grammar kinds and field names are language-specific. Arbitrary upstream AST constructors with embedded snippets (for example `Call(name=\`x\`)`) remain unsupported.

A regex is a predicate, not a top-level structural search: it is syntactically accepted as a `query` so it can occur after `<:`, but a top-level regex has no candidate text and produces `PATTERN_INVALID_CONTEXT`. `empty` is valid only as a direct `<:` right-hand side; elsewhere it is a syntax error.

### 1.1 Identifiers and metavariables

Metavariable names are ASCII and case-sensitive. `$x` and `$X` differ. `$_` is the anonymous wildcard: every occurrence is independent, it is never recorded, may not be used on the left of `<:`, and does not participate in equality. Names beginning with a digit, non-ASCII names, bare `$`, and names beginning with `_` are rejected. Identifiers written literally in snippets follow the selected target parser's rules, independently of metavariable names.

Inside a snippet, `$name` is recognized before target-language parsing. A backtick snippet is decoded by replacing `\`` with a backtick and `\\` with a backslash; every other backslash escape is invalid. Regex escapes are decoded as listed by the grammar and the result is compiled with Go's RE2 syntax. Regex matching is unanchored unless the author writes anchors. It is applied to the exact UTF-8 source bytes covered by a binding, converted to a string; a list binding uses the single span from its first element's start through its last element's end, including intervening source. An empty list supplies the empty string.

## 2. Go snippet parsing

After metavariables are replaced by typed placeholders, a snippet is tried in this fixed context order:

1. one Go expression;
2. one Go type;
3. one Go statement;
4. a non-empty Go statement list;
5. one Go declaration;
6. a non-empty Go declaration list;
7. a complete Go file, including its `package` clause.

A context succeeds only when all decoded snippet input is consumed. Role inference is structural, not a scoring search: one skeleton parse represents every occurrence by a source-ordered unique generic identifier. Ordinary roles come from that identifier's actual Go ancestor/field, while whole import specs, import alias/path pairs, grouped declaration specs, type specs, and top-level declarations are resolved only inside their grammar-local component. Import components have at most the grammar's one alias and one path. The lexical component scan ignores literals and same-line comment punctuation, but records newlines inside block comments because Go treats them as semicolon-insertion separators. The pinned Go grammar is deterministic, so current inference produces exactly one assignment and performs one authoritative whole parse after the skeleton parse. Recovery nodes are evidence for locating a component, not alternate interpretations. There is no global role product, greedy repair, or Hamming-occurrence search. Grammar-component alternatives are represented component-wide (including both members of an import alias/path pair) so the validation path can accept multiple complete assignments defensively if a future grammar or edge case derives them. At most one assignment is emitted per component, so a context performs at most `metavariable occurrence count + 2` whole parses including the skeleton parse; current inference performs two. The number of ordinary placeholders is bounded only by the pattern byte limit.

The first successful context wins. The current deterministic Go grammar yields one inferred interpretation at that priority. The implementation nevertheless restores and canonicalizes every grammar-derived assignment supplied by inference: duplicate restored templates count once, and more than one distinct template produces the defensive `PATTERN_AMBIGUOUS_SNIPPET` diagnostic. Error-recovery readings are not candidates. Lower contexts are never considered after a valid candidate. A bare identifier is therefore an expression, and an expression statement uses the expression interpretation. Package fragments, unbalanced fragments, metavariables embedded in literals/comments or adjacent without grammar punctuation, and placeholders used where the selected Go grammar category cannot occur are rejected.

Synthetic parse scaffolding is not part of the resulting template. Authored statement and declaration lists use explicit `statement_sequence` and `declaration_sequence` template roots (including a one-item sequence when an explicit semicolon must be retained); only an authored complete file uses `source_file`. Every decoded metavariable occurrence must restore to exactly one slot before a candidate is accepted. A metavariable occupying an entire repeated grammar field is `SlotMany`, even when only one value is written. This includes direct name fields under `var_spec`, `const_spec`, `parameter_declaration`, and `field_declaration`, direct type fields under `type_case`, and a whole `type_elem` under an interface (so it is ready to bind either method-like or type-union interface elements), in addition to explicit list-node children. Type/name fields in those same ancestors remain singular when the grammar field itself is not repeated.

The supported Go grammar version is declared by the implementation for `gritql-v1`; that exact version must be reported in evaluation metadata and must not vary during one evaluation.

### 2.1 JavaScript, TypeScript, and TSX snippet parsing

For `language javascript`, the contract selects `.js` and `.jsx` source; `language typescript` selects `.ts`, `.mts`, and `.cts`; `language tsx` selects `.tsx`. Snippets are parsed as expression, statement, statement list, declaration, declaration list, and complete-file contexts using the matching pinned Tree-sitter grammar. TypeScript and TSX additionally support type contexts. Unlike Go's deterministic first-context interpretation, every grammar-valid JavaScript/TypeScript-family interpretation is retained: for example, `Promise<$type>` can represent both a TypeScript instantiation expression and a generic type, and either source shape may match.

Synthetic wrappers are removed before matching. Statement and top-level declaration lists use `statement_sequence` and `declaration_sequence`; TypeScript/TSX class-member snippets use a grammar-local class wrapper and `list_sequence`. Repeated fields and unfielded children come from parser-generated metadata for the selected grammar, including arguments, parameters, object members, class members, statements, and declarations. Explicit semicolons remain structural; omitting one in a snippet does not match a source statement containing one.

JavaScript/TypeScript-family metavariables occupy identifier-like, expression, property, JSX tag/attribute, parameter, class-member, and repeated-list grammar positions; TypeScript and TSX additionally support type positions. Import-source placeholders are quoted only when grammar context requires a module string: a default-import identifier immediately after `import` remains an identifier, while a placeholder after `from` or in a side-effect import is a source. Whole-snippet placeholders retain explicit declaration alternatives for syntax that cannot safely be represented by one identifier node. Unsupported positions fail compilation with `PATTERN_INVALID_SNIPPET`; they are never interpreted by a fallback parser. JavaScript, TypeScript, and TSX metadata reports canonical `language` and `grammar` fields instead of the Go-only `go_grammar` field.

### 2.2 Python snippet parsing

For `language python`, the contract selects `.py`, `.pyi`, and `.pyw` source and parses snippets against the pinned Python grammar. It retains every grammar-valid expression, statement, statement-list, declaration, declaration-list, and complete-module interpretation. Statement and declaration sequences share a `statement_sequence` projection because Python modules and blocks contain the same statement grammar; matching remains confined to one module or block and never crosses indentation scopes.

Python metavariables occupy expression, pattern, identifier, dotted import-name, statement, declaration, and grammar-generated repeated-list positions. A placeholder immediately following `from` or `import` binds one complete `dotted_name`. Whole-snippet placeholders receive explicit statement and declaration interpretations so they can bind syntax that cannot be represented by an identifier expression. Indentation, delimiters, operators, and literal spelling remain structural, while formatting trivia follows the shared normalization rules. Unsupported or malformed positions fail with `PATTERN_INVALID_SNIPPET` rather than falling back to text or another grammar. Python metadata reports canonical `language`, `grammar`, and pinned `tree_sitter_grammar` fields.
### 2.3 C, C++, C#, Dart, Java, Kotlin, Rust, and Shell snippet parsing

C, C++, C#, Dart, Java, Kotlin, and Rust use grammar-local synthetic wrappers to parse expression and statement snippets without retaining wrapper syntax. Their adapters also retain statement-list, declaration, declaration-list, and complete-file interpretations. C#, Dart, and Java additionally try class-member declaration contexts. C and C++ select `.c`/`.h` and the documented C++ extensions respectively; C#, Dart, Java, Kotlin, and Rust use the canonical extensions in the capability matrix. Shell selects `.sh`, `.bash`, and `.zsh` and supports command/statement, command-list, function/assignment declaration, and complete-script contexts; it does not invent a separate expression grammar.

Metavariables occupy identifier-like and grammar-generated repeated positions. Wrapper selection requires one exact named syntax node, and sequence selection requires complete consecutive children within one grammar block or source root. Generated wrappers, helper declarations, and delimiters outside the authored snippet are clipped before freezing the template. Every interpretation must parse without recovery nodes and restore every metavariable exactly once. Unsupported grammar positions fail compilation rather than falling back to text, regex, or another target language.

All adapters report their canonical language ID, grammar ID, and pinned Tree-sitter implementation in evaluation metadata. Query algebra, equality, transactional findings, resource limits, acquisition, ordering, and diagnostics are identical across target languages.

## 3. Structural matching and bindings

A source file is parsed into the selected language's lossless concrete syntax tree. A structural value is normalized recursively as:

- the node kind;
- every ordered named child and unnamed punctuation/operator/token child;
- for a leaf token, its token kind and exact UTF-8 lexeme.

Whitespace, line terminators, comments, and automatically inserted semicolons are trivia and are omitted. Explicit semicolons, parentheses, delimiters, operators, identifier spelling, literal spelling, and escape spelling remain. No formatting, Unicode, numeric, or string-literal canonicalization occurs. Two node bindings are structurally equal exactly when these normalized trees are equal. Two list bindings are equal exactly when their lengths match and corresponding elements are structurally equal. This definition is used both for repeated names and branch merges. A node range starts at its first non-trivia token and ends after its last non-trivia token, so leading and trailing trivia never enters a match range; trivia between those tokens remains in the returned source slice.

A named metavariable's first occurrence binds the subtree permitted by its snippet grammar position. Later occurrences of that name must be structurally equal. A metavariable in an entire repeated-child slot may bind a consecutive list of zero or more children; it cannot splice through punctuation or across different parents. Repeated-list equality compares only corresponding normalized elements; separators are never equality members, so a trailing comma does not distinguish `f(a,)` from `f(a)`. Separators authored as literal template/node syntax, including explicit semicolons, remain structurally significant. List choice is greedy: try the longest sequence first, then shorter sequences, while alternatives and later snippet elements are tried left to right. The first complete structural match wins for that candidate. This deterministic backtracking rule also applies to multiple list metavariables. Bindings retain source order and are scoped to one candidate evaluation. Every list binding has one overall span for its grammar slot plus one ordered, non-overlapping element range per structural element. A non-empty overall span starts at the first normalized element and ends after the last normalized element, including intervening trivia and separators but excluding leading and trailing separators. An empty list has no element ranges and has a zero-width overall span immediately before the next concrete token, or immediately after the previous concrete token when there is no next token.

A `where` block is evaluated after its prefix succeeds. Constraints are evaluated left to right. Its left side must already have a binding from the prefix or a committed earlier constraint; this v1 grammar has no construct that can create such a binding solely on a left side. `$x <: P` runs `P` with `$x` as its candidate (or each list element in source order, succeeding once for each successful element). For `r"..."`, it instead tests the binding text as specified above. `$x <: empty` succeeds exactly once when `$x` is a bound list with **zero structural elements**; it fails for non-empty lists and scalar node bindings, even if their text is empty. It tests structural cardinality, not trimmed source text. Every constraint must succeed.

For example, `language go` followed by `` `for { $body }` where { $body <: empty } `` detects an empty Go loop body without matching a function body. Other languages may use an argument-list metavariable such as `` `target($args)` where { $args <: empty } ``; which syntactic positions admit a zero-length list is determined by that language's pinned grammar and adapter. `parent kind("function")` checks **only the immediate syntax parent** of the current node (or the list's grammar parent for a list candidate). It recognizes the parser's pinned function-like node kinds, including methods and supported anonymous function forms; it does not look through intermediate blocks, infer types, or resolve calls. It preserves the current match range and bindings, so `not parent kind("function")` excludes only direct function bodies. Only the `"function"` category is supported.

### 3.1 Cross-file relations (versioned hook runner)

The `gritql-relational-v1` hook engine joins findings from **complete** local source snapshots; it is not the standalone unsupported `multifile` query expression. A rule compiles `left_query` and optional `right_query` (defaults to the left query) and `partition_query` as ordinary language-qualified GritQL programs. `left_key` and `right_key` select named scalar node bindings; optional `descendant_kind` chooses the first matching syntax descendant, useful for unwrapping a Go pointer/generic receiver. Keys compare normalized structural values, not source whitespace. `scope` is `directory` or `repository`; an optional per-file partition query/key prevents matches across namespaces or packages. A right finding is reported when a matching left finding is in **another file**. `unique_left: true` suppresses ambiguous declarations; false reports duplicate occurrences across files, with one result per right location. All programs in a relation must target the same language. Source diagnostics, truncation, missing/ambiguous partitions, and missing required bindings are errors, not clean results. `relation.left_include` optionally narrows **reported left declarations** by repository-relative glob without narrowing the right reference universe or skipping validation of scanned files. `relation.max_findings` optionally raises the per-program finding bound from 10,000 to at most 100,000; exceeding that explicit bound still fails closed.

For example, `variable_declarator(name=$name)` on both sides with `scope: repository` finds TypeScript declarations repeated between files:

```yaml
version: 1
id: duplicate-symbol
event: Stop
engine: gritql-relational-v1
include: ["**/*.ts"]
severity: warning
message: "duplicate {{key}} also appears in {{left.basename}}"
relation:
  left_query: |
    language typescript
    variable_declarator(name=$name)
  left_key: {binding: name}
  right_key: {binding: name}
  scope: repository
```

`relation.mode: unmatched_left` reports each **left** finding whose key has no right-side match in the same scope/partition, including the same file. The default mode retains cross-file pair behavior. This is a bounded, source-authored anti-join over the entire selected snapshot; `unique_left` and `{{right.*}}` message placeholders are invalid in unmatched mode. `{{left.*}}` and `{{key}}` describe the reported left finding. Missing sources, parser diagnostics, resource limits, and missing/ambiguous partitions still fail closed; absence in a partial scan is never a clean result.

`relation.mode: unmatched_left_any` with `left_key: {binding: result, projection: go-return-types}` evaluates **all top-level declared Go result types** of each left finding. It reports the left finding only when none match a right-side named interface declaration in the same directory and `package_clause` partition. A missing optional result binding (for a function with no result) also reports it; direct inline interfaces and predeclared `error`/`any` satisfy the check. The projection accepts direct named and generic named results, not pointers, slices or qualified imported results. It does not type-check aliases, shadowed predeclared identifiers, imported interfaces, or build-tag configurations; unresolved names remain warnings. Use `maybe function_declaration(result=$result)` to capture optional returns. See [the standalone Go function warning hook](../.grepple/hooks/go-standalone-functions.yaml). Relational hooks scan the complete selected snapshot even without `--all`; `relation.report_changed_only: true` limits **reported** findings to Git-changed paths by default while still resolving against all eligible files. `--all` reports every finding.

[The installed Go method candidate hook](../.grepple/hooks/go-uncalled-methods.yaml) (also available as a [reusable example](../examples/go-uncalled-methods.yaml)) authors its two selectors in GritQL: `method_declaration(name=$name) where { $name <: r"^[a-z]" }` (ASCII-unexported declarations) and `selector_expression(field=$name) where { $name <: r"^[a-z]" }` (any same-name selector reference), joined by name within a directory and source-written Go package. It excludes `**/*_test.go` from **both** declaration and reference scans: a production method referenced only in tests becomes a candidate, while methods declared in tests are not checked. Run it with `grepple hook --id go-uncalled-methods --all`; a relational rule forces a complete scan of included sources even without `--all`. It flags **no matching selector name in the selected source universe**, not proven dead code: method values are included, unrelated fields or another receiver with the same name can hide candidates, and unresolved dynamic calls, reflection, generated files, build-tag variants, excluded sources, and external uses can create false alarms. Review each warning before removal. No type, receiver, dispatch, or reachability inference is implied.

The installed [private Go type rule](../.grepple/hooks/go-dead-private-types.yaml) joins source-written `type_spec` names with type-identifier references in the same directory and package. The [exported internal type](../.grepple/hooks/go-dead-internal-types.yaml) and [exported internal method](../.grepple/hooks/go-dead-internal-methods.yaml) rules limit **declarations** to `internal/**/*.go` but scan references from all included Go files, including `cmd/`. Each excludes `*_test.go`, so a test-only reference does not suppress a candidate. The type rules use `type_identifier() as $name` and exclude each declaration's own name with `not within type_spec(name=$name)`. These are name-based candidates, not proof of dead code: different packages/receivers with the same name can hide findings, self-references and method receivers can mask dead types, and interface calls (such as `ServeHTTP`, `Unwrap`, and `UnmarshalText`), reflection, code generation, and external users can cause false alarms. Review warnings before removal.

`.grepple/hooks/same-file-struct-methods.yaml` instead joins Go `type_spec` nodes to `method_declaration` receivers, partitioned by `package_clause`. Configured `include`/`exclude` globs constrain the source universe. Choosing a relation forces `grepple hook` to scan all selected repository sources even without `--all`, because unchanged declarations can affect changed methods. Complete results are cached by selected source bytes, rule definition, and executable; incomplete results are never cached.

### 3.2 Source-authored scoped metrics

`gritql-metric-v1` accepts a **GritQL metric document**, not a Go implementation of a rule. Its top-level map contains one `metric` map; `scope`, `base`, `above`, and nonempty `rules` are required. Optional `name` identifies the scope's grammar name field, and `boundary` skips nested subtrees. Each rule has `id`, GritQL `query`, and nonnegative `points`; optional `depth` multiplies enclosing nesting, `opens: true` increases nesting for descendants, `flat` suppresses an else-if nesting increase, `logical: "each"|"runs"` and `operators` count short-circuit tokens or operator runs, `child` requires a named child kind, and `self` compares a direct call field to the source-written scope name. All selectors use the document's language header and the closed `gritql-v1` selector contract. Unknown map fields, duplicate IDs, invalid grammar kinds/fields, malformed sources, truncation, or exhausted budgets are errors, never clean scores. The calculation traverses the source tree under one shared step/time budget and returns a score with a per-event breakdown for each scope.

The installed [McCabe hook](../.grepple/hooks/go-mccabe.yaml) defines `scope: or { function_declaration(), method_declaration() }`, `base: 1`, `above: 10`, and GritQL rules for `if`, `for`, non-default switch/type-switch/select arms, and `&&`/`||`. Multiple case labels with one body count as one arm; nested function literals are excluded. A separate [cognitive example](../examples/go-cognitive.yaml) authors Revive-style structural weights in GritQL. Direct recursion currently compares **lexical names**, not resolved Go object identities, so shadowing can differ from Revive. The existing Revive cognitive-complexity rule remains enabled; see the [Revive parity report](gritql-metric-parity.md) before enabling that example.

The installed [nested-loop rule](../.grepple/hooks/go-nested-loops.yaml) selects Go `for_statement()` nodes and scores each inner loop once per enclosing loop via `points: 0, depth: 1, opens: true`. It warns only when the function score is **above 1**, meaning at least two inner-loop occurrences: a single outer/inner pair scores 1 and stays quiet, while two separate pairs score 2 and a three-deep chain scores 3. It includes range loops and loops separated by an `if`, but excludes nested function-literal bodies. Sequential loops score zero. The score is the sum of enclosing-loop depths, **not an estimate of time complexity**: it does not know bounds, input sizes, whether loops run, or call costs. The warning is located at the named function or method.

Metric hooks use changed-file discovery when selected alone; they report functions strictly **above** `above`, with `{{name}}`, `{{score}}`, and `{{above}}` message placeholders. Selecting a repository relation alongside them (as the default set does here) forces a complete scan, including unchanged files. Use `--id go-nested-loops` or `--id go-mccabe` for changed-file scope. Selected source is content-validated, and only complete per-file scans are cached. Metric scans currently run serially, independent of `--workers`. The Go API `CompileMetric` and `AnalyzeMetrics` can run the same authored document without a hook. Metric documents are not ordinary `grepple grit` search patterns; the ordinary `gritql-v1` contract remains closed.

## 4. Evaluation model

The evaluator visits named source AST nodes in preorder: parent before children, children in grammar order, with equal-offset children in grammar order. At every repeated-child position in the selected pinned grammar (identified by parent kind plus field name, including an empty field name), it also visits each non-empty consecutive element list exactly once, longest first and then by increasing first-element index. This includes statements, declarations, arguments, parameters, fields, repeated names, and equivalent adapter-supported list positions. A list candidate's range runs from its first normalized element through its last and excludes leading or trailing separators. Empty lists are bindable while matching a surrounding snippet but are not traversal candidates.

Queries return zero or more match records. Each record contains a primary range and a binding map:

- `parent kind("function")` succeeds once only when the current candidate's direct grammar parent is function-like, retaining the candidate range and bindings. An ancestor at greater depth does not suffice.
- A snippet matches the current candidate structurally and returns that candidate's range.
- A regex or `empty` predicate is valid only as a direct `<:` right-hand side; a successful constraint retains its prefix match range.
- `and { A, B, ... }` evaluates every member against the same candidate. It forms the left-to-right Cartesian join of successful records, retaining only compatible bindings. Its range is the smallest byte span covering all member ranges.
- `or { A, B, ... }` evaluates every branch against the same candidate in source order and concatenates their records in branch order. It does not stop at the first success.
- `not P` succeeds once, with the current candidate's range and no new bindings, exactly when `P` returns no record.
- `maybe P` returns `P`'s records when any exist. If none exist, it succeeds once with the current candidate's range and no new bindings. Absence is not represented by a null or empty binding.
- `contains P` evaluates `P` at the current candidate and every named descendant in preorder. Each success retains `P`'s range and bindings.
- `within P` evaluates `P` at the current candidate and then each named ancestor, nearest first. Each success keeps the original current candidate's range, while retaining compatible bindings from `P`.
- A postfix `where` filters its prefix records and preserves each prefix range.

Containment is therefore reflexive: a node contains and is within itself. A *strict descendant* or *strict ancestor* excludes that node; v1 has no spelling that requests strict containment. For list candidates, containment uses the lowest common parent: self means that exact list, descendants are named nodes inside its elements, and ancestors begin at the common parent.

Each prefix, block branch, constraint, `not`, and `maybe` is a binding transaction. Failure rolls back all writes. Successful `and` members commit compatible unions; conflicting repeated bindings reject only that joined record. Each `or` branch starts from the same incoming map and commits only into its own records. `not` always discards child bindings. A successful `maybe` commits its child bindings, while its absent case commits none. No binding leaks between top-level candidates, findings, or files.

A top-level query runs once at every traversal candidate. `not` and absent `maybe` can consequently produce findings when used at top level; authors should normally use them inside `and` or `where`. Every returned top-level record becomes a finding using the composition range above.

## 5. Source, paths, ranges, ordering, and deduplication

Source must be valid UTF-8. Offsets are zero-based UTF-8 **byte** offsets into the unmodified file; ranges are half-open `[start_byte,end_byte)`. Lines and columns are one-based; a column is one plus the number of Unicode scalar values since the line start, not a byte count or display-cell width. LF advances one line. In CRLF, the pair advances one line and neither scalar belongs to the next line. Offset/range conversion must use the original bytes. Returned matched text is exactly that byte slice.

Input paths are converted to repository-relative slash-separated paths by replacing the host platform's separator with `/`, removing `.` segments, and resolving `..` only when it remains inside the repository root. Slash input is canonical on every host. Backslash is a separator and drive prefixes are rejected on Windows only; on POSIX, backslash and colon are ordinary filename characters. Absolute paths, paths escaping the root, NUL, and empty normalized paths are rejected. No case folding or Unicode normalization is performed. Path and string collation is lexicographic order of unsigned UTF-8 bytes.

A finding has `path`, `start_byte`, `end_byte`, `start_line`, `start_column`, `end_line`, `end_column`, `pattern_id`, `message`, and `bindings`. `bindings` is serialized by metavariable name in UTF-8 byte order. A node binding contains `kind`, one `range`, and one normalized structural node. A list binding contains `kind`, one overall `range`, ordered `ranges` for its elements, and an equally sized array of normalized structural nodes.

Before output, findings are sorted by this complete key:

1. normalized path;
2. `start_byte`;
3. `end_byte`;
4. `pattern_id`;
5. message;
6. canonical binding serialization (UTF-8 JSON with sorted object keys, no insignificant whitespace);
7. canonical complete finding serialization as the final total-order tie-breaker.

Two findings are duplicates only when their canonical complete finding serializations are byte-identical. Adjacent duplicates are removed after sorting. Line/column values never participate independently because byte offsets determine them.

### 5.1 CLI local-plus-remote composition

`grepple grit` and explicit `--local` evaluate only the current directory. Explicit `--remote` or `--server` evaluates the local directory and queries the authenticated router, then applies one client-side normalization and paging pass. When the checkout's Git remote can be canonicalized as `owner/repository`, that identity is sent in `exclude_repositories`; findings attributed to it are also discarded client-side in case a backend ignores the selector. Findings from unrelated repositories are retained even when their paths match local paths.

The composed ordering prepends repository identity to the v1 finding key, normalizes path separators, and then uses range, pattern identity, stable binding content, and the complete finding as deterministic tie-breakers. Only byte-identical complete findings are removed. User `skip` and `limit` are applied once to this global sequence. The client fetches bounded remote pages only until it has enough candidates for that window, and each request respects the `MaxGritPageLimit` server cap.

Statistics are added with integer saturation. Metadata must agree, while diagnostics, truncations, and shard errors from successful responses are retained. A successful response with shard errors is therefore a renderable partial result; transport, authentication, cancellation-before-transport, and incompatible-metadata failures remain command errors.

## 6. Errors and diagnostics

Pattern lexing/parsing, unsupported syntax, invalid context, and invalid regex reject the entire pattern and produce no findings from it. A source parse error rejects that entire file, emits exactly one `SOURCE_PARSE` diagnostic for the parser's earliest error (lowest byte offset, then parser message byte order), and produces no partial finding for that file. Other files may still be evaluated.

Diagnostics are separate from findings and contain these required fields: `code`, `class`, `severity` (`error`), `message`, `pattern_id` (string or null), `path` (normalized string or null), and `range` (the same byte/line/column fields as a finding, or null). Messages may improve, but code and class are stable API. Diagnostics are sorted by path (null first), range start (null first), code, pattern ID (null first), and message, all string comparisons using unsigned UTF-8 bytes.

Every `PATTERN_*` diagnostic and `LIMIT_PATTERN_BYTES` identifies its submitted pattern with a non-null `pattern_id`, has a null `path`, and has a non-null range into that pattern's query source. The deterministic v1 convention is to report the whole submitted pattern, including the `language go` header: `[0,len(pattern UTF-8 bytes))`, with scalar-based line and column coordinates computed exactly as for source files. This convention also applies when the error is localized or the byte limit is crossed earlier. Resource and runtime diagnostics other than `LIMIT_PATTERN_BYTES` may have a null range.

Stable v1 codes are:

| Code | Class | Meaning |
| --- | --- | --- |
| `PATTERN_PARSE` | `pattern` | malformed token stream or EBNF violation |
| `PATTERN_UNSUPPORTED` | `unsupported` | recognized out-of-contract construct |
| `PATTERN_INVALID_CONTEXT` | `pattern` | valid token used in a forbidden context |
| `PATTERN_INVALID_SNIPPET` | `pattern` | snippet is not valid Go in any supported context |
| `PATTERN_AMBIGUOUS_SNIPPET` | `pattern` | multiple distinct grammar-derived assignments at the selected snippet priority (defensive) |
| `PATTERN_INVALID_REGEX` | `pattern` | invalid RE2 expression or regex limit exceeded |
| `SOURCE_INVALID_UTF8` | `source` | source is not valid UTF-8 |
| `SOURCE_PARSE` | `source` | Go source parse failure |
| `PATH_INVALID` | `source` | path cannot be normalized safely |
| `LIMIT_PATTERN_BYTES` | `resource` | pattern byte limit exceeded |
| `LIMIT_SOURCE_BYTES` | `resource` | source byte limit exceeded |
| `LIMIT_PARSE_DEPTH` | `resource` | parse/tree depth limit exceeded |
| `LIMIT_CANDIDATES` | `resource` | traversal candidate limit exceeded |
| `LIMIT_AST_STEPS` | `resource` | AST/match step budget exceeded |
| `LIMIT_FINDINGS` | `resource` | rule finding limit reached |
| `LIMIT_TIME_FILE` | `resource` | per-file deadline exceeded |
| `LIMIT_TIME_BATCH` | `resource` | batch deadline exceeded |
| `LIMIT_MEMORY` | `resource` | accounted memory limit exceeded |
| `EVALUATION_CANCELLED` | `cancelled` | caller cancellation observed |
| `INTERNAL_ERROR` | `internal` | evaluator invariant or unexpected failure |

Unsupported constructs should use `PATTERN_UNSUPPORTED`, not `PATTERN_PARSE`, when the lexer can identify them. Internal errors must not expose secrets or stack traces in `message`.

### 6.1 Compile explanation

`grepple grit explain QUERY` compiles under the same bounded native contract without acquiring or evaluating source files. Human output is bounded by `--max-output-bytes`; `--json` emits complete `grepple-grit-explain-v1` compile output under the selected pattern, regex, instruction, and parse-depth limits.

A successful explanation includes the resolved language and `gritql-v1` compatibility, grammar ABI/fingerprint, used feature names, every grammar-valid snippet wrapper interpretation (expression/type/statement/declaration/file context and concrete root kind or root slot), and named metavariables in first-occurrence order. Variable entries report source occurrence count, node/list binding cardinalities, wrapper root/field roles, expression roles, and constraint-left roles. These roles describe compiled syntax positions, not type resolution or runtime data flow.

A failed explanation contains one bounded structured compile diagnostic with stable code/class, severity, message, and source range when available. It emits no partial program interpretation.

## 7. Resource bounds and partial results

Library and server evaluations use the defaults below. A host may lower them, but may not exceed the hard maxima while claiming bounded v1 execution. The interactive local CLI is intentionally exempt from wall-clock defaults: it has no implicit per-file or batch deadline, while retaining cancellation and all deterministic count, size, and memory limits. Local users can opt into deadlines with `--max-file-time-ms` and `--timeout-ms`; explicit deadlines still use the hard maxima.

| Resource | Default | Hard maximum |
| --- | ---: | ---: |
| decoded pattern body | 256 KiB | 1 MiB |
| one source file | 10 MiB | 50 MiB |
| pattern/Go tree depth | 256 | 1,024 |
| traversal candidates per file | 250,000 | 1,000,000 |
| AST/match steps per file | 10,000,000 | 100,000,000 |
| findings per rule per batch | 10,000 | 100,000 |
| per-file wall time, including source acquisition | 2 s | 10 s |
| batch wall time, including source acquisition | 30 s | 300 s |
| eligible files per filesystem scan | 100,000 | 1,000,000 |
| acquired source bytes per filesystem scan | 1 GiB | 16 GiB |
| filesystem scanner workers | 4 | 64 |
| accounted live memory per evaluation | 128 MiB | 512 MiB |
| decoded regex length | 16 KiB | 64 KiB |
| RE2 compiled program instructions | 100,000 | 500,000 |

The evaluator uses Go RE2 semantics; matching is linear-time and no regex timeout or backtracking engine is permitted. Regexes that exceed the byte or compiled-program bound fail during pattern validation with `PATTERN_INVALID_REGEX`.

Files are evaluated sequentially in normalized path order for contractual output, even if an implementation computes speculatively. A file is transactional: source parse failure, candidate/depth/memory/file-time limit, cancellation observed during that file, or internal error discards every finding from that file. Previously committed files remain except for caller cancellation or a batch timeout, which returns **no findings at all**. This all-or-nothing cancellation rule makes its result independent of scheduling and checkpoint timing.

The finding cap is applied separately to each `pattern_id` in the globally sorted, deduplicated stream: retain the first N findings for each rule and emit one pathless `LIMIT_FINDINGS` diagnostic for every truncated rule. Retained findings and all diagnostics are globally resorted and deduplicated before publication. Oversized/invalid files produce their diagnostic and no findings, then evaluation continues. A batch memory failure discards the current file and stops after previously committed files. Other batch resource failures stop before the next normalized path. Caller cancellation or batch timeout discards all findings, retains already-determined non-cancellation diagnostics, and adds exactly one sorted, deduplicated cancellation/timeout diagnostic. No speculative or half-evaluated record is returned.
Implementations must check cancellation and deadlines before every file, at least every 1,024 traversal candidates, and before committing a file. `LIMIT_CANDIDATES` counts only the named-node and consecutive-list traversal candidates described above; it does not count AST inspection, query operations, constraints, or matcher backtracking. Those operations consume the separate AST/match step budget and report `LIMIT_AST_STEPS`. Accounted memory includes parsed pattern/source trees, candidate state, bindings, findings, and regex programs; exceeding it must be detected at allocation-accounting boundaries rather than relying on process OOM behavior.

The filesystem scanner accepts repository-scoped candidate paths rather than discovering files itself. It normalizes and canonical-deduplicates those paths, retains only candidates matching the compiled program's target language, applies include globs followed by exclude globs, and processes survivors in normalized path order. An absent language is inferred from the canonical parser extension registry; an unsupported or mismatched language, unmatched glob, file missing a mandatory anchor, or file containing a NUL byte is skipped without diagnostics. Files are acquired with a bounded read of at most the lower source/memory allowance plus one detection byte; `os.ReadFile`-style unbounded acquisition is forbidden. Worker count is bounded and may be reduced below the requested/default count by the effective memory allowance. Every anchor-selected non-binary file is parsed once, and its tree is closed after evaluation.

`max_files` and `max_total_bytes` scanner truncations are reported separately from language/glob/binary skips and from evaluation diagnostics. Their stable records contain `reason`, effective `limit`, and the count of normalized eligible paths omitted. File-count truncation retains the first normalized paths. Aggregate-byte truncation commits only the longest normalized prefix whose acquired bytes fit. Worker completion order does not affect findings, diagnostics, statistics, or truncation records. Scanner cancellation and batch timeout follow the same all-or-nothing finding rule as batch normalization and may return before an in-progress filesystem read finishes because a generic `fs.FS` read is not interruptible.

Candidate optimization is conservative. Eligible anchors are exact significant snippet leaf tokens containing at least one Unicode letter, number, or underscore. `and` and mandatory structural `where` constraints combine their anchors; `or` retains only anchors shared by every branch; `contains` and `within` preserve their operand anchors; and `not`, `maybe`, and regex constraints contribute none. A query with no eligible mandatory literal has no safe anchor and requires the complete scoped file set. The scanner requires the longest reported anchor during its single bounded acquisition pass before parsing; mixed-program scans apply each program's anchor independently before sharing a language-matched parse. Anchor optimization changes acquisition and parse volume only, never findings.

## 8. Explicitly unsupported syntax and behavior

The following are recognized but unsupported and fail closed with `PATTERN_UNSUPPORTED`:

- rewrites or replacement arrows such as `=>`;
- pattern definitions, named definitions, and calls to them;
- imports, modules, libraries, and remote patterns;
- standalone `multifile` and `sequential` query expressions (cross-file joins are supported by the separate relational hook engine);
- arbitrary upstream AST constructors with snippets or calls (the bounded node-pattern subset above is supported);
- language prefixes or qualifiers on snippets/patterns (the only language syntax is the required header);
- `as` captures;
- assignments, predicates/functions, custom or foreign functions;
- any equality or inequality operator (including `==` and `!=`); equality exists only through repeated metavariable binding;
- any operator, literal, comment form, or delimiter absent from the EBNF.

Targets outside the generated capability matrix; type checking; name resolution; data flow; network access; shell execution; repository writes; interactive input; and source rewrites are behaviorally unsupported. The unified contract is native and detection-only.

## 9. Security and performance gates

The kernel is a library-only detector. Production code in `gritql` and its parser facade performs no network access, process execution, dynamic extension loading, or source writes. It reads only caller-supplied bytes or explicitly scoped `fs.FS` candidates. Queries cannot invoke foreign functions, imports, shell commands, or remote registries. Paths and globs are validated before source acquisition. Regexes use bounded RE2 compilation, evaluation loops charge explicit candidate/step/time/memory budgets, scanner acquisition is bounded, and compiled-program caches contain immutable IR only—never source ASTs.

The following Linux/amd64 baseline was recorded on 2026-09-08 using an Intel i7-1165G7 and `-benchtime=100ms`:

| Benchmark | Time/op | Bytes/op | Allocations/op | Review threshold |
| --- | ---: | ---: | ---: | --- |
| compile representative query | 0.31 ms | 49 KB | 1,149 | investigate above 1 ms, 100 KB, or 2,000 allocs |
| evaluate representative file | 0.63 ms | 212 KB | 2,381 | investigate above 2 ms, 500 KB, or 5,000 allocs |
| shared parse, 100 files / 2 rules | 69 ms | 23.6 MB | 260,936 | investigate above 200 ms, 50 MB, or 500,000 allocs |
| anchored scan, 100 files | 41 ms | 12.6 MB | 135,887 | investigate above 120 ms, 30 MB, or 300,000 allocs |
| unanchored scan, 100 files | 51 ms | 16.1 MB | 170,855 | investigate above 150 ms, 40 MB, or 400,000 allocs |
| pre-cancelled 100-file scan | 17 µs | 21 KB | 143 | investigate above 200 µs, 100 KB, or 500 allocs |

Thresholds are review gates rather than portable pass/fail assertions: CPU, Go version, tree-sitter build, and race instrumentation affect absolute numbers. The invariant suitable for CI is that the shared two-rule scan parses exactly 100 files, not 200. Run the reproducible gates with:

```bash
go test ./gritql -run '^$' -bench=. -benchmem
for target in CompileNoPanicDeterministic CompileSnippetNoPanicDeterministic BindingRollbackNoPanicDeterministic EvaluateFileNoPanicDeterministic NestedTraversalNoPanicDeterministic ValidateGlobsNoPanicDeterministic; do
  go test ./gritql -run '^$' -fuzz="^Fuzz${target}$" -fuzztime=10s
done
```

Representative scan benchmarks supplement the 100-file microbenchmarks with deterministic 500-file medium and 5,000-file large repository shapes. They distribute files across every registered target language, nested module/package paths, varied source sizes, and one structural finding per file. `cold-compile-and-scan` includes query compilation; `warm-precompiled` reuses immutable programs. Separate benchmarks report sampled Go heap peaks as `peak-heap-bytes/op` and measure active cancellation from the first filesystem read to transactional return as `cancel-ns/op`:

```bash
go test ./gritql -run '^$' -bench '^BenchmarkRepresentative' -benchmem -benchtime=1x
```

The peak metric samples `runtime.MemStats.HeapAlloc` every 100 microseconds and is not an operating-system RSS measurement. Benchmark corpora are deterministic and network-free; absolute values remain review evidence rather than portable assertions.

## 10. Conformance and versioning

A conforming implementation must fixture-test every grammar production, supported snippet context, binding transaction, range rule, diagnostic code, ordering key, limit outcome, and unsupported category. The versioned `language-reliability` fixture additionally requires every registered target language to cover named/list metavariables, deterministic snippet-context selection (including competing interpretations where its grammar permits them), malformed query and source syntax, cancellation, and a resource-limit outcome. Its coverage test is derived from `SupportedLanguages`, so registering a language without all required vectors fails the suite. `gritql-v1` metadata publishes the canonical target language and grammar identifier, every effective limit, and the pinned Tree-sitter implementation identity for Go callers.

The `hook-selectors` conformance fixture exercises source-authored `gritql-v1` hooks (node/field selectors, capture, scoped exclusion, immediate-function parent checks, empty lists, and import placeholders), including Dart whole-node bindings. A separate versioned `hook-engines` fixture runs relational joins, unmatched-left and unmatched-left-any modes against complete source snapshots and checks source-authored McCabe and nested-loop metric scores, thresholds, and fail-closed partial scans. These augment, rather than replace, the per-language reliability vectors and focused engine unit tests.

The supported language set remains closed. Additive read-only syntax is documented and fixture-tested within this `gritql-v1` implementation (including structural node patterns); changing established matching, range, ordering, cancellation, or diagnostic behavior requires a new compatibility contract. The separate `gritql-relational-v1` hook engine versions cross-file semantics explicitly. Existing stable codes may not be reassigned.
