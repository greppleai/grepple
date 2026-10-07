# Language-independent hook string assertions

A file-local `gritql-v1` YAML rule may select syntax nodes and assert equality
between two bounded string expressions. Compliant matches produce no findings;
unequal values produce the normal rule finding at the matched node. This is
hook-level evaluation, not an extension to the GritQL query grammar. The engine
contains no package, component, group or architecture naming policy.

```yaml
version: 1
id: declaration-path-name
engine: gritql-v1
include: ["src/*/*/*.go"]
severity: error
unsuppressible: true
message: "expected {{expected}}, got {{actual}} in {{path}}"
query: |
  language go
  package_clause($name)
assert:
  equals:
    - binding: name
    - concat:
        - segment: {input: {path: true}, separator: "/", index: 1}
        - segment: {input: {path: true}, separator: "/", index: 2}
```

`src/catalog/controller/main.go` thus requires `catalogcontroller`. Selectors
remain language-specific; assertions do not. For JavaScript, for example, use
`language javascript` with `identifier() as $name` and a `.js` include glob. The
same path/string expression compares the captured identifier. Select only the
syntax you intend to validate; absence of a selected node is not itself a finding.

## Expression vocabulary

Every expression mapping contains exactly one operator:

| Operator | YAML value | Meaning |
| --- | --- | --- |
| `literal` | string | Constant, including an empty string |
| `binding` | bare capture name | Exact source text of that query binding |
| `path` | `true` | Normalized repository-relative slash-separated file path |
| `concat` | nonempty expression list | Concatenate in listed order |
| `lower`, `upper` | expression | Unicode case conversion |
| `trim` | expression | Remove surrounding whitespace |
| `basename`, `dirname` | expression | Slash-path base/directory operations |
| `replace` | `{input: EXPR, old: STRING, new: STRING}` | Literal replace-all; `old` must not be empty |
| `segment` | `{input: EXPR, separator: STRING, index: INT}` | Split on a nonempty literal separator; zero-based index, negative indexes count from the end |

`assert.equals` requires exactly two expressions. Message placeholders are
`{{actual}}`, `{{expected}}`, and `{{path}}`; expansion is nonrecursive. Captures
must be declared in the query, present on each evaluated match, and wholly inside
the matched source range. A capture outside that range is an error, not a lexical
fallback or a new filesystem read. No type resolution, string-literal unescaping,
filesystem lookup, executable expressions, scripts or shell commands are used.

## Safety and integration

Unknown YAML fields/operators, malformed expressions, unsupported placeholders,
missing bindings, out-of-range segments, parser diagnostics and resource limits
fail closed. Assertions are currently file-local only and cannot be combined
with `annotation`; relational and metric engines reject `assert` configuration.

Bounds: depth 8 (root depth 0), 64 expression nodes across both operands, 32 KiB
literal/intermediate/result/message text, separator length 128 bytes, and a
32 MiB shared assertion-finding byte ceiling. Replace/concat/message expansion
is checked before allocating the expanded value. Existing structural scan limits
still apply. Assertions preserve the native changed-file/full-scope behavior,
parallel parsing, source selection, suppression/unsuppressible rules and cache
invalidation. Rule expressions are included in cache signatures; moved files
are evaluated under their new path. Verification never changes source.

Repository-specific policies belong to that repository's `.grepple/hooks/`.
See [native structural grammar and source selection](gritql-compatibility.md).