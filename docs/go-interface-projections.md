# Source-declared Go interface projections

Grepple's `go-standalone-functions` warning remains unchanged: its legacy
`go-return-types` projection exempts predeclared `error` and `any`, operates within
a directory/package partition, and does not resolve imported interfaces.
Two optional native relational projections provide stricter source-based checks.
They implement reusable Go syntax operations; repository API policy stays in YAML.

Both require `relation.go_module`, `scope: repository`, a Go source collector
(`partition_query: language go; source_file() as $source`), and
`partition_key: {binding: source}`. Write that query as a YAML block with the
language directive on its own line. The source collector must select complete
source files. Selected-file scan diagnostics, missing snapshots, malformed Go,
ambiguous source packages and unsupported dot imports fail closed. `left_include`
filters only reported declarations, never the interface/source universe.

## `go-interface-returns`

Use `mode: unmatched_left_any`, `left_key: {binding: result, projection:
go-interface-returns}`, and a right query `type_spec(name=$type,
type=interface_type())` with `right_key: {binding: type}`. A typical left query is:

```gritql
language go
and {
  function_declaration(name=$name),
  maybe function_declaration(result=$result),
} where { $name <: r"^\\p{Lu}" }
```

Each selected function must declare at least one nonempty literal interface
result or a source-declared interface product. A companion `error` result does
not exempt concrete results. `error`, `any`, unnamed/named empty interfaces,
pointers, slices, channels and functions containing interfaces are not products.
Named generic interface instantiations are supported. Imported interfaces are
resolved by exact module-relative source directory, actual source package name
and explicit/default import names—not by coincidentally matching type names.
Dependencies outside the selected module are not resolved; expose an owned
interface or enlarge the source-authored contract rather than assume a type.

## `go-method-signatures`

Use `mode: unmatched_left`, `left_key: {binding: name, projection:
go-method-signatures}`, a left query `method_declaration(name=$name)` (optionally
filtered to Unicode exported names), and a right query `method_elem(name=$name)`
with `right_key: {binding: name}`.

Methods are compared by name, ordered parameter/result types, multiplicity of
named fields and variadic markers. Parameter names are ignored. Package-local
named types and explicitly qualified types retain package identity; imported
aliases are normalized. Default external package names use the import basename
(with a trailing Go version directory removed); use explicit import names when
that convention does not identify the dependency. `Error() string` is recognized
as the predeclared Go `error` interface method; similarly named incompatible
signatures are not exempt.

This is source-signature membership, not whole-program Go type checking. It does
not prove that a receiver implements every member of an interface, resolve
indirect aliases/embedded-interface expansion, infer constructor allocation or
model build-tag selection. Generic method type substitution and structurally
complex anonymous types may require review. Use compiler-checked constructor
returns or explicit conformance assertions and normal builds/tests as the final
implementation check. A matching signature in an unused interface is not proof
of a meaningful architectural boundary.

Both projections use the existing bounded, read-only snapshot, reporting,
suppression/unsuppressible and cache machinery. They do not rewrite source or
execute Go source. See [native hook configuration](gritql-compatibility.md).