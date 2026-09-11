# GritQL integration analysis

_Analyzed on 2026-09-07._

This is a historical design analysis, not the current API specification. The public-core refactor moved the parser and API to `parser/` and `api/`, renamed `internal/grepplecli` to `internal/cli`, and moved router/shard ownership to the separate `grepple-backend` repository. Backend proposals below do not imply that those implementations ship in this module. The original Go-only recommendation has since been preserved as `gritql-go-v1` and extended through separate TypeScript/TSX adapters under `gritql-v1`. See `gritql-compatibility.md` for the implemented contracts.

## Executive recommendation

Grepple should adopt a **native Go-only GritQL-compatible detection kernel** and define the supported surface explicitly in `docs/gritql-compatibility.md` (`gritql-go-v1`).

This means:

- parse and evaluate a constrained Go structural-query subset in-process
- expose deterministic diagnostics for Grepple's existing rule and hook pipeline
- **do not** depend on an external GritQL engine, Node runtime, native addon, or fallback runner path
- keep the kernel read-only and suitable for lint-style detection first; rewrites can remain out of scope until detection is proven

The evidence below still matters: GritQL-style structural matching is a strong fit for Go code policies, the upstream Go target has had parse-quality gaps, and packaging/distribution for external runners has been inconsistent. Those facts argue for a Go-native kernel, not for adopting an external engine.

## Potential use cases

| Use case | Value | Recommendation |
| --- | --- | --- |
| Repository-specific structural lint rules | High | First target |
| Tested, documented autofixes | High | Later, only after detection is trusted |
| Dependency and API migrations | Very high | Support once the kernel is stable |
| Organization-wide structural rules | Very high | Good long-term fit |
| Live interactive search | Medium | Optional later |
| Replacing Grepple's regex/Zoekt engine | Low | Do not pursue |
| Reimplementing unrelated GritQL features not covered by the contract | Low | Do not pursue |

## Repository-specific code-quality checks

Grepple currently uses:

- `gofmt`
- Revive with the pinned rules in `revive.toml`
- The custom `same-file-struct-methods` analyzer
- Generated architecture-schema validation
- CodeQL and Trivy in CI

A Go-native GritQL-compatible kernel would complement these tools with declarative, syntax-aware policies such as:

- Server packages must not import `internal/grepplecli`.
- Production subprocesses should use `exec.CommandContext`.
- HTTP clients must declare a timeout.
- Request paths should propagate context instead of creating `context.Background()`.
- Selected error-returning calls must not be discarded.
- Deprecated Fiber, Zap, or tree-sitter APIs must not be introduced.
- API migrations must update calls and surrounding imports together.

An illustrative lint-only pattern is:

```yaml
version: 0.0.2

patterns:
  - name: require_context_for_exec
    title: Require context-aware subprocesses
    level: warn
    body: |
      language go
      `exec.Command($args)`
    description: |
      Prefer exec.CommandContext when subprocess cancellation should
      follow the caller.
```

This should initially report findings without rewriting code. Replacing `exec.Command` automatically is unsafe unless a pattern can prove that an appropriate context variable exists.

## Codemods and dependency upgrades

Grepple can eventually use the same structural contract for coordinated edits, but only after the detection kernel is stable. Likely follow-on uses include:

- Upgrading Fiber APIs across packages
- Changing logging conventions
- Migrating deprecated Go APIs
- Rewriting configuration formats
- Applying coordinated edits across multiple files

Grepple should identify affected repositories and files first, then let a separate, reviewed workflow decide whether to apply edits.

## Organization-wide structural rules

Grepple's saved rules already provide most of the required control plane:

- Router-owned rule definitions
- Distribution to shards
- Backfill across indexed repositories
- Re-evaluation after repository indexing
- Materialized counts and file lists
- Commit and evaluation timestamps

A future structural rule could conceptually use this model:

```json
{
  "id": "require-http-timeout",
  "name": "HTTP clients require timeouts",
  "engine": "gritql",
  "compatibility": "gritql-go-v1",
  "mode": "files",
  "pattern": {
    "language": "go",
    "body": "...",
    "severity": "error",
    "message": "HTTP client has no timeout"
  }
}
```

The existing `RuleRepoResult` counts and paths are sufficient for an initial dashboard. Rich reporting would eventually require findings containing:

- File path
- Start and end positions
- Message and severity
- Matched source
- Optional replacement preview
- Pattern version
- Evaluated commit

## Proposed integration architecture

### Native kernel only

The right architecture is a small, deterministic evaluator implemented in Go:

- lex/parse pattern snippets in-process
- resolve metavariables and boolean/query operators
- walk Go source with stable source-range reporting
- emit findings in a structured format that Grepple can store and sort
- keep the execution path free of external process dependencies

### Placement in Grepple

A Go-native structural evaluator can be used in three places:

1. local lint-style checks
2. Stop-hook diagnostics for fast, high-confidence rules
3. shard-side materialized rule evaluation for repository-wide scans

For each case, Grepple should filter by repository, glob, and language before structural matching, apply strict limits, and sort results deterministically before persistence.

## Relationship to existing tools

| Existing capability | GritQL-compatible kernel relationship |
| --- | --- |
| Revive | Adds project-specific structural rules; does not replace Go linting |
| CodeQL | Adds fast syntactic policies; does not replace semantic data-flow security analysis |
| Trivy | Adds source policies; does not replace image, dependency, or configuration vulnerability scanning |
| Zoekt and regex search | Adds structural precision after candidate discovery; should not replace indexing |
| Custom tree-sitter analyzers | Makes simpler policies easier to author; complex semantic invariants may remain better in Go |

The kernel is primarily syntax-aware. It should not be treated as a substitute for Go type checking or full interprocedural analysis.

## Current blockers and risks

### Go support is alpha upstream

The upstream Go target has been described as alpha-level, and prior tests showed that some current Grepple files produced parse errors even when simple queries like `exec.Command(...)` were matched successfully. Grepple uses Go 1.25, so complete parsing must be an adoption gate for the contract.

This is a strong reason to keep the contract narrow and well-tested in Go rather than relying on a moving external runtime.

### Standard-library patterns require curation

The GritQL standard library contains many patterns, but only a small Go subset is likely useful here. At least one Go autofix concerns loop-variable capture semantics that changed in Go 1.22 and is not generally appropriate for this Go 1.25 project.

Do not enable broad imported pattern sets without explicit review.

### Security and resource isolation

Treat patterns as executable policy rather than harmless configuration:

- remote modules can be fetched from Git repositories
- some engines support network access and external functions
- rewrites can produce invalid or unsafe source
- an untrusted pattern could consume substantial CPU or memory

For shard-side execution:

- only administrators should define structural rules
- pin module hashes or equivalent content identifiers
- run read-only, with resource limits and no repository credentials
- never commit or push a rewrite automatically

## Adoption gates

Before enabling the kernel in `make lint`, the Stop hook, or production:

1. **Parsing:** 100% of supported Grepple Go files parse without diagnostics.
2. **Precision:** At least 95% of reviewed findings are true positives.
3. **Recall:** Every golden positive fixture is detected.
4. **Performance:** Warm local checks finish within a few seconds, and shard evaluation stays within a defined CPU and memory budget.
5. **Determinism:** Repeated runs return identically ordered findings.
6. **Reproducibility:** Offline builds exist for Linux amd64 and arm64.
7. **Output stability:** Structured ranges and rule identities can be consumed without parsing human-readable output.
8. **Security:** Verification performs no network access, external commands, or source writes.

## Recommended next step

Implement the native `gritql-go-v1` detection kernel and validate it with a golden fixture corpus against three initial project-owned patterns:

1. Prevent server packages from importing `internal/grepplecli`.
2. Flag production use of `exec.Command` for review.
3. Require timeouts on HTTP clients.

The proof of concept should measure parsing coverage, range stability, ordering, and runtime. It should not depend on an external engine or fallback runner.

## Sources

- [GritQL overview](https://docs.grit.io/)
- [GritQL configuration](https://docs.grit.io/guides/config)
- [Continuous integration](https://docs.grit.io/guides/ci)
- [Testing GritQL](https://docs.grit.io/guides/testing)
- [Target languages](https://docs.grit.io/language/target-languages)
- [GritQL repository](https://github.com/biomejs/gritql)
- [GritQL standard library](https://github.com/biomejs/gritql-stdlib)
