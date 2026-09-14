# Boundary-analysis improvements

The current boundary report combines repeated file-owned workflows with concrete type spread. That was enough to expose broad `go-tree-sitter.Node` use, but reaching the stronger conclusion—"the language adapter should be the entry point and generic engines should consume a project-owned syntax abstraction"—still required manual architectural interpretation.

This document records additional checks that could make that conclusion easier to reach. These checks should report evidence and confidence rather than label findings as violations. Function placement and abstraction quality are semantic design questions; pretending they can be proved from syntax alone would make the checker noisy and brittle.

## 1. Third-party representation permeability

Report where an imported concrete type appears relative to its first project-owned wrapper or facade.

Useful facts:

- production and test files using the concrete type;
- parameters, results, receivers, fields, locals, aliases, and embeddings;
- exported/public signature exposure;
- project-owned wrappers containing the type;
- packages and directories reached beyond the wrapper owner;
- the ratio of raw-type consumers to wrapper consumers;
- newly reached files or packages in a semantic diff.

Example evidence:

```text
go-tree-sitter.Node
  raw reach: 24 production files
  wrappers: parser.Node, parser.ViewNode
  wrapper reach: 9 production files
  public raw exposure: parser.NavigationGraphFromTree
```

This is a strong signal when the raw type crosses packages or public APIs. Within an implementation package it is only a lead: adapters may legitimately need the backend representation.

**Implementation difficulty:** medium. Import-qualified parameter/result/local facts already exist, but fields, aliases, embeddings, and wrapper containment require declaration-level type facts.

**Fragility:** low for resolved imports and public signatures; medium for inferred wrapper relationships; high in dynamic languages without explicit types.

## 2. Backend-type allowlists and containment zones

Allow architecture policy to declare where a concrete dependency is permitted:

```text
github.com/tree-sitter/go-tree-sitter.*
  allowed: parser/tree_sitter_backend.go
  allowed: parser/language_*.go (temporary)
  forbidden: exported signatures
  forbidden: search/**, extract/**, api/**
```

The report should show facts without policy, while an optional check mode can evaluate repository-owned rules. A semantic diff should highlight the first use in a new zone.

**Implementation difficulty:** low once canonical type identities and field facts exist.

**Fragility:** low when paths and import identities are explicit; medium when policies depend on filename conventions that may change during refactors.

## 3. Generic-engine syntax-policy leakage

Detect grammar-kind comparisons in files designated as generic engines. Examples include:

```go
switch node.Kind() {
case "field_identifier", "command_name":
}
```

Useful evidence:

- grammar-kind literals compared against `Kind()`;
- field-name literals passed to `ChildByFieldName`;
- the languages whose generated grammar metadata contains each literal;
- whether the containing function receives a language/navigation adapter;
- whether equivalent classification methods already exist on the adapter;
- whether adding a language has historically required editing the generic file.

Generated grammar metadata can distinguish likely syntax identifiers from ordinary strings. The check should not ban all field names in shared code: genuinely cross-language structural conventions exist.

**Implementation difficulty:** medium. Tree-sitter-backed structural queries can locate comparisons, and generated grammar metadata can classify literals. Determining whether a function is semantically generic requires policy or ownership metadata.

**Fragility:** medium. Grammar kind names are explicit and deterministic, but shared names such as `identifier`, `body`, and `name` can be intentionally generic. Results need confidence levels, not binary failure by default.

## 4. Adapter bypass analysis

For a function that accepts both a syntax node and an adapter, report direct syntax interpretation that could have been delegated. Signals include:

- `Kind()` comparisons in a generic function;
- direct import, parameter, binding, visibility, or call classification;
- language-family terms in function names;
- calls to low-level node helpers without any adapter call in the same decision path;
- edits to a generic function occurring alongside new-language support.

A high-confidence candidate has all of these:

1. it lives in a configured generic-engine file;
2. it accepts or can reach an adapter;
3. it branches on grammar metadata;
4. multiple literals belong to different language grammars.

**Implementation difficulty:** high. Syntax can identify calls and branches, but proving that a branch should be delegated requires control-flow and ownership context.

**Fragility:** medium to high. Useful as ranked review evidence; too error-prone as an unconditional CI failure.

## 5. Misplaced-function affinity

Rank functions whose dependencies have stronger affinity with another file or concern than with their declaring file.

Potential features:

- percentage of callees owned by another file;
- types used primarily by another owner;
- shared naming prefix or receiver/container ownership;
- grammar literals associated with one language adapter;
- callers concentrated in one external file family;
- a function called only by one adapter but stored in a generic file;
- source-history co-change affinity, when Git history is available;
- whether moving the function would remove a dependency edge or cycle.

Example:

```text
addTypeScriptNavigationImports
  declared: parser/navigation_context.go
  only caller family: TypeScript navigation adapter
  syntax literals: import_specifier, namespace_import
  suggested affinity: parser/language_navigation.go or TypeScript adapter file
```

The checker should say "placement candidate", never "misplaced", unless an explicit repository policy establishes ownership.

**Implementation difficulty:** high. Call/type/literal affinity is feasible; robust concern inference and move suggestions are substantially harder.

**Fragility:** high without declared ownership zones. Utility files, generated code, tests, and intentionally shared helpers otherwise create many false positives.

## 6. File cohesion and utility-hub classification

The current workflow analysis can mistake intentionally shared utility files for leaked workflows. Add cohesion facts:

- external callable surface ratio;
- proportion of declarations used externally;
- number of unrelated caller clusters;
- type/topic clusters among declarations;
- whether the file mostly contains stateless helpers;
- whether calls are one-off utility calls or repeated multi-step protocols.

A file with 10/11 externally used callables is probably a utility surface. It should not be ranked like a component whose private workflow escaped through several consumers. Conversely, repeated multi-step protocols around a small private surface remain strong extraction candidates.

**Implementation difficulty:** medium for structural metrics; high for semantic topic clustering.

**Fragility:** low for surface ratios; medium for caller clustering; high for inferred topics.

## 7. Repeated protocol analysis around a type

Combine type spread with member and call order:

```text
Node protocol
  NamedChildCount -> NamedChild
  ChildByFieldName -> Text
  Kind -> language-specific switch
```

This distinguishes harmless pass-through parameters from repeated interpretation logic. Report protocols by containing function, file, and concern zone, preserving candidate confidence.

**Implementation difficulty:** medium. Member/call facts and source order already exist, but receiver resolution must preserve canonical imported type identity.

**Fragility:** medium. Fluent APIs and ordinary collection iteration can look like protocols; require cross-file repetition and show exact evidence.

## 8. Parallel-abstraction detection

Report project-owned wrappers and the concrete type they encapsulate when both remain broadly consumed.

Useful questions:

- Does `Node` or `ViewNode` contain `sitter.Node`?
- Which consumers use the wrapper, the raw type, or both?
- Are raw consumers concentrated in adapters, or spread into generic engines?
- Does the wrapper expose enough operations used by raw consumers?
- Is there more than one wrapper with overlapping methods and different lifetime semantics?

This would have highlighted `Node`, `ViewNode`, and raw `sitter.Node` as parallel syntax representations rather than merely three popular types.

**Implementation difficulty:** high. Field containment is straightforward, but API-overlap and "wrapper completeness" need member-shape comparison and call-site analysis.

**Fragility:** medium. Wrapper containment is reliable; inferring that two types are competing abstractions is heuristic.

## 9. Public dependency-chain exposure

Trace public signatures transitively:

```text
public API -> project type -> field/method signature -> third-party type
```

Direct exposure is high confidence. Transitive exposure should distinguish exported representation from private implementation fields.

This can identify a public project wrapper that still leaks its backend through one method, even when no caller directly imports the backend elsewhere.

**Implementation difficulty:** medium once complete declaration/type facts exist.

**Fragility:** low for statically resolved public signatures; medium for aliases and generic constraints; high for dynamic languages.

## 10. Architectural change budgets

Static spread often reflects intentional historical architecture. Semantic diff thresholds are more actionable:

- no new public third-party type exposure;
- no new package or directory reached by a raw backend type;
- no new grammar-kind branch in a generic engine;
- no increase in raw-to-wrapper consumer ratio;
- no new adapter bypass candidate above a confidence threshold.

Baselines should be explicit and reviewable. Cache state and platform must not affect results.

**Implementation difficulty:** medium because semantic graph diffing already exists.

**Fragility:** low for additive type/package facts; medium for heuristic placement or adapter-bypass scores.

## Recommended implementation order

1. Add declaration-level type roles for fields, aliases, embeddings, and interface methods.
2. Distinguish third-party, same-module, standard-library, and project-owned imported types.
3. Add direct public and transitive public exposure checks.
4. Add optional type containment-zone policies and semantic diffs.
5. Add grammar-literal evidence using generated metadata.
6. Add repeated protocols around canonical receiver types.
7. Experiment with function-affinity and parallel-abstraction ranking in report-only mode.
8. Promote only low-fragility checks to CI failures; keep semantic placement suggestions advisory.

The guiding rule is to make hidden coupling visible without claiming that popularity, utility reuse, or a syntactic similarity proves an architectural defect.
