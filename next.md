# Next improvements

`solved.md` records completed capabilities. This file contains only open work and the evidence needed to prioritize it.

## Recommended next milestone

> Verify generated and queried output determinism across supported operating systems, using normalized architecture comparison to localize drift.

## Planning principles

- Prefer a new usable capability over endless syntax-resolution refinement after deterministic candidates, bounded output, and safe binding lifetime are established.
- Revisit resolution completeness when measured ambiguity blocks an actual task.
- Human output must remain bounded and disclose omissions. Complete machine output must remain available, but large results may be delivered through a disclosed file artifact rather than injected into stdout/context.
- Syntax-based navigation, structural matching, architecture relations, and boundary findings are evidence—not compiler, runtime, type-flow, or policy verdicts.
- Preserve dependency direction toward `api` and `parser`; keep GritQL query semantics out of `parser`.
- Keep canonical language IDs and grammar-kind policy out of generic navigation engines.

## Reliability and compatibility gates

- [ ] Add larger golden fixtures for every supported language.
- [ ] Cover malformed, nested, generic, decorated/annotated, and multiline declarations per language.
- [ ] Expand CRLF, symlink, build-constraint, ambiguous-extension, and Windows/cross-platform path cases.
- [ ] Run fuzz targets longer in scheduled CI and retain minimized failures as seeds.
- [ ] Verify generated and queried output determinism on Linux, macOS, and Windows; compare normalized semantics first and raw bytes second, retaining the first source-linked failure as CI evidence.
- [ ] Define schema compatibility and deprecation rules for public `api` DTOs and versioned CLI JSON before the next public release.
- Keep test, race, vet, lint, schema, architecture benchmark, and agent benchmark gates green after each change.

### Delegated research performance

Live `grepple ask` dogfooding showed two distinct bottlenecks: ordinary local/remote investigations took about 55 seconds while tool execution consumed only 0.85–3.09 seconds, so model round trips dominated; a subprocess audit spent 41.58 of 60.11 seconds in tools because two unnecessary whole-repository navigation builds took 14.13 and 27.15 seconds.

- [ ] Add a focused `inspect_symbol` tool that can combine bounded symbol discovery, deterministic declaration selection, immediate related navigation, and exact source retrieval in one call, reducing common count → files → snippets → navigate → read sequences.
- [ ] Add an `auto` `search_code` projection that returns aggregate concentration, top matching paths, bounded snippets, and exact next locations together while retaining explicit `count`, `files`, and `snippets` modes.
- [ ] Detect repeated or increasingly broad equivalent research calls and return cached evidence plus a synthesis/narrowing hint. Do not reintroduce a model-step cutoff; the overall timeout remains the execution bound.
- [ ] Reduce repeated model-input tokens by activating only the relevant typed-tool subset after initial intent becomes observable, while ensuring local, remote, architecture, and exact-location questions can still reach every required tool.
- [ ] Add parsed-versus-restored file counts and universe build/restore durations to ask performance logs once parser-document restoration is implemented; keep token chunks omitted.
- [ ] Add answer-gated ask benchmarks for local ownership, subprocess/security audit, graph impact, structural matching, and exact remote-version research. Gate on answer correctness, citation validity, ref fidelity, completeness disclosure, elapsed time, model steps, tool calls, tool time, and tokens rather than latency alone.
- [ ] Set regression targets from the live baseline: eliminate full-graph work for invalid navigation locations, avoid repeated identical scans within one ask, and reduce routine source-backed investigations from multi-call discovery chains to one or two evidence calls before synthesis.

### Navigation and evidence calibration

- [ ] Add adapter-owned import-relation parity for other supported languages where syntax and repository layout permit conservative local target resolution. Extend C/C++ beyond exact source-relative quoted includes only when explicit compiler or build evidence provides include-search paths.
- [ ] Add cross-file navigation goldens for aliases, receivers, overload-like declarations, generics, nested scopes, and ambiguous imports in every language that claims the relevant adapter capability.
- [ ] Measure resolved-local, ambiguous-local, unresolved-local, and expected-external outcomes on representative repositories instead of treating candidate counts as precision.
- [ ] Add answer-gated tasks that intentionally exercise reflection, dynamic dispatch, generated code, registration, and build-system edges; verify Grepple reports uncertainty rather than unsupported architectural claims.
- [ ] Document when users must hand evidence to a compiler, language server, build-system query, or runtime tool for refactor safety; do not imply Grepple alone proves exact semantic impact.
- [ ] Low priority, evidence-triggered: improve ambiguous navigation resolution only when representative tasks show that conservative candidate or unresolved edges materially block work. Keep improvements adapter-owned and syntax- or manifest-evidenced; do not pursue compiler-grade whole-program analysis, dynamic-runtime inference, or framework guessing merely to increase edge counts.

### Package-manager production hardening

The first exact external-resolution matrix covers Go modules, npm lockfiles, Cargo registry locks, and direct Maven coordinates. Production support must model the package manager's resolved graph rather than treating a manifest version string, repository tag, or terminal import name as sufficient identity.

**Cross-ecosystem invariants**

- [ ] Define a versioned dependency-evidence contract containing ecosystem, package coordinate, resolved version, source/registry identity, integrity/checksum, replacement kind, target conditions, owning workspace member, and the manifest/lock location that supplied each field.
- [ ] Distinguish declared constraints from selected versions. Never send ranges, dynamic versions, inherited properties, unresolved variables, or floating Git references as exact artifact evidence.
- [ ] Make source identity part of matching. The same name/version from a public registry, private registry, Git repository, vendored tree, local path, mirror, or replacement is not automatically the same artifact.
- [ ] Stop assuming package versions map to Git tags. Registry tarballs, crate archives, Maven coordinates, Git revisions, monorepo release tags, subdirectory packages, and generated source bundles need explicit provenance links before source navigation is called exact.
- [ ] Preserve multiple exact dependency candidates when source syntax cannot identify artifact ownership. Package/class/module membership in the indexed artifact must disambiguate; terminal-name, group-prefix, repository-name, and popular-package guesses must not.
- [ ] Model nested modules and monorepos explicitly. Manifest ownership must follow the nearest applicable workspace/package root, while an indexed repository artifact may publish many independently versioned modules.
- [ ] Apply target and configuration conditions: operating system, architecture, runtime, build profile, feature set, optional dependency activation, test/dev scope, and generated-source selection. If the active condition is unknown, retain conditional candidates rather than asserting one edge.
- [ ] Define integrity policy per source kind. Missing checksums, mutable snapshots, local paths, Git branches, and registries without trusted integrity evidence need explicit weaker/unresolved states instead of sharing registry-lock confidence.
- [ ] Handle lockfile schema/tool-version evolution with bounded parsers, fixtures from real package-manager versions, unknown-field tolerance, deterministic ordering, and explicit unsupported-version diagnostics.
- [ ] Treat malformed, partially merged, stale, platform-specific, and manifest/lock-mismatched files as non-authoritative. Qualification failure must preserve the original unresolved syntax evidence.
- [ ] Resolve private registries and repositories through repository-aware authorization. Never leak global artifact existence, credentials, package URLs containing secrets, or cross-tenant cache hits.
- [ ] Add acquisition policy for an exact dependency that is not indexed: authorization, allowlists, rate/concurrency limits, immutable fetch keys, checksum verification, retry/backoff, quarantine, and observable unresolved reasons.
- [ ] Separate package artifact identity from source artifact identity. Record how a package archive/JAR/crate maps to a repository commit and path; do not infer that mapping solely from package metadata or a matching version label.
- [ ] Define pruning and invalidation for manifests, lockfiles, resolved snapshots, package archives, source artifacts, and dirty-worktree overlays. Resolver inputs—not only source bytes—must participate in cache keys.

**Go modules**

- [ ] Cover minimal-version selection across the complete module graph, multiple `require` blocks, indirect requirements, `exclude`, `retract`, tool dependencies, `vendor/modules.txt`, and workspace module selection.
- [ ] Preserve exact semantics for module-path/version suffixes, submodules, pseudo-versions, versioned and local `replace`, root and nested `go.work`, private proxies, `GONOSUMDB`/`GOPRIVATE`, and alternate `GOPROXY` sources.
- [ ] Validate module zip/checksum provenance rather than assuming `go.sum` proves a GitHub repository/tag mapping; distinguish module and `/go.mod` hashes and handle sum-database-disabled private modules conservatively.

**Node/npm**

- [ ] Complete explicit Node lockfile and package-manager compatibility without parsing one format as another.
  - [x] Support `package-lock.json` lockfile versions 1–3 with version-specific top-level layouts; malformed, missing-version, and unsupported-version locks remain non-authoritative.
  - [x] Support `npm-shrinkwrap.json` lockfile versions 1–3 with npm precedence over `package-lock.json`; an invalid selected shrinkwrap remains non-authoritative instead of falling back.
  - [x] Honor exact Corepack `packageManager` selections: exact npm versions allow npm locks, while non-npm, floating, and malformed selections prevent stale npm locks from qualifying dependencies.
  - [ ] Not yet supported: Yarn classic/Berry, pnpm, and Bun lockfiles.
- [ ] Model workspaces, hoisting, nested `node_modules`, peer dependencies, optional dependencies, bundled dependencies, overrides/resolutions, and file/link/workspace/Git/tarball sources.
- [x] Preserve npm import aliases (`npm:`), including scoped targets, separately from the exact resolved package identity in lockfile versions 1–3.
- [ ] Resolve package subpaths through `exports`, `imports`, `main`, `module`, `types`, `typesVersions`, conditional exports, and runtime/module conditions. A package name alone does not prove which source declaration an import reaches.
- [ ] Enforce `os`, `cpu`, engine, and optional-install conditions when they affect the selected graph.
- [ ] Verify registry URL and SRI integrity for exact archives; custom registries and mirrors must not silently collapse into the public npm identity.

**Rust/Cargo**

- [ ] Support renamed dependencies, workspace-inherited dependencies, target-specific tables, optional dependencies/features, dev/build dependencies, multiple versions of one crate, alternate registries, registry source replacement, `[patch]`, Git revisions, and path dependencies.
- [ ] Distinguish package name from crate/library target name and map `use` paths through `lib.name`, module declarations, re-exports, enabled features, and generated code before resolving source declarations.
- [ ] Treat Cargo checksums as registry-package evidence only. Git/path packages require exact commit or local snapshot identity, and absence of `Cargo.lock` for libraries must remain an explicit completeness limitation.

**Maven, Gradle, Java, and Kotlin**

- [ ] Build an effective Maven model: parent POMs, properties, profiles, dependency management, imported BOMs, transitive dependencies, exclusions, scopes, optional dependencies, relocation, repositories/mirrors, and reactor modules.
- [ ] Include Maven type/classifier and snapshot timestamp/build identity. `groupId:artifactId:version` alone is insufficient for classifiers, platform artifacts, test fixtures, multi-release JARs, and mutable `-SNAPSHOT` versions.
- [ ] Add Gradle support separately rather than treating Gradle files as Maven syntax: Groovy/Kotlin DSL, version catalogs, dependency constraints/platforms, lockfiles, dependency verification metadata, substitutions, composite builds, included builds, variants, capabilities, and dynamic versions.
- [ ] Do not infer Maven/Gradle ownership from Java/Kotlin package prefixes. One artifact may contain many packages and split packages may appear in many artifacts; exact indexed archive/source membership must perform final disambiguation.
- [ ] Account for Kotlin metadata, multiplatform source sets, JVM names, generated declarations, and Java/Kotlin mixed modules when mapping a selected artifact to source.

**Production verification**

- [ ] Keep hermetic provider/consumer fixtures for every ecosystem, generated by real package-manager versions where possible. Cover exact resolution, wrong version, missing lock, stale lock, local/path replacement, aliases, multiple exact providers, conditional dependencies, private/custom registries, and corrupt integrity.
- [ ] Add HTTP-level multi-shard E2E tests through authenticated `/public/navigation/resolve` and artifact download, including candidate merging, immutable provenance, source bodies, unavailable shards, authorization denial, and deterministic repeated results.
- [ ] Add representative real-package fixtures pinned to immutable archive/checksum and source-commit identities, while keeping network access out of ordinary unit tests.
- [ ] Emit per-ecosystem telemetry for references observed, exactly qualified, conditionally qualified, unresolved by reason, artifact missing, version/source mismatch, ambiguous providers, and successfully source-resolved declarations.

### Structural-query reliability

- [ ] Expand `gritql-v1` conformance fixtures across every supported language for named/list metavariables, ambiguous snippet contexts, malformed syntax, cancellation, and resource limits.
- [ ] Benchmark structural scans on representative medium and large repositories, including peak memory, cancellation latency, and cold/warm behavior.

## Output and CLI consistency

- [ ] Reconcile or clearly document the different output-mode contracts: search human/JSON/JSON-matches, graph JSON-or-compact, boundaries human/JSON, extract Mermaid, and GritQL human/JSON.
- [ ] Explain JavaScript-regex defaults at first use and in examples.
- [ ] Make limit units explicit: files, findings, candidates per section, nodes, bytes, and source files.
- [ ] Distinguish local and remote feature availability at the attempted command.
- [x] Clarify that graph `dependencies`/`dependents` currently describe navigation edges rather than a build-system import graph.
- [ ] Separate resolution outcomes from confidence labels: report resolution, ambiguous-local, unresolved-local, and expected-external rates. The current `candidate` outcome and `candidate` confidence use different meanings, while ambiguity rate alone hides a large unresolved population.
- [ ] Keep complete JSON available for scripts, but direct agents toward filtered projections and artifact descriptors rather than multi-megabyte graph/boundary documents.
- [ ] Remove guidance drift such as duplicate workflow steps and completed milestones still named in the recommendation; add lightweight documentation consistency checks.

## Operational and release readiness

- [ ] Run the `v0.0.1` Release Please flow on GitHub and verify all five native hosted-runner archives, checksums, conditional attestation behavior, and downloaded-binary smoke tests; extend smoke coverage beyond `version` to recursive help, local search, architecture comparison, and one GritQL query.
- [ ] Test concurrent processes writing the same content-addressed artifact or cache entry, interrupted writes, corrupt entries, cleanup during reads, and permission preservation.
- [ ] Measure cold/warm runtime, peak RSS, cache size, artifact disk growth, and cleanup behavior on representative medium and large repositories.
- [ ] Define supported Go versions, operating systems, repository-size expectations, schema support windows, and release rollback/migration behavior.
- [ ] Add explicit local-only, authentication-required, backend-unavailable, and unsupported-remote command tests so deployment availability is observable rather than inferred.

## Parser and cache direction

- [ ] Make parser documents the reusable cache boundary: a live Tree-sitter tree or versioned packed read-only CST behind one lightweight accessor.
- [ ] Parse or restore each file once, then derive outlines, navigation, extraction, and multiple GritQL evaluations.
- [ ] Keep packed per-file facts path-neutral; apply repository, module, directory, and configured-ignore context after loading.
- [ ] Invalidate by source digest, language, grammar fingerprint/ABI, schema, and parser/extractor version.

## Language expansion

- Keep Shell focused extraction unsupported unless dogfooding defines a useful command/script projection.
- [ ] Add new grammars in priority order: Terraform/HCL, Swift, Dart, then PHP. Apply current capability-parity, help-discoverability, malformed-source, and cross-platform determinism gates to each language before treating it as production-ready.
- [ ] Prioritize later grammars such as Ruby, Scala, Protocol Buffers, SQL, Lua, and Elixir from measured user demand and repository dogfooding rather than declaration syntax alone.

## Success measures

- A new user can discover and run search, graph impact, boundaries, directory architecture, and one GritQL query using recursive CLI help only.
- An agent can move from omitted related edges to a complete focused query by copying one suggested command.
- Every bounded response or spilled artifact descriptor identifies the bound/delivery decision, omitted work, evaluated source universe, and completeness path.
- Boundary output separates third-party permeability from first-party reuse, standard-library spread, test-only use, and approved internal infrastructure.
- Warm repeated graph queries parse or restore every unchanged file at most once and exactly match cold output.
- Dogfood benchmarks measure answer correctness, false architectural claims, retrieval turns, stdout/context bytes, artifact reads, peak memory, and cold/warm cache behavior as well as runtime and allocations.
- Per-language capability reports distinguish unsupported analysis from attempted-but-unresolved evidence.
- The same complete report generated on Linux, macOS, and Windows is normalized-semantically equal; byte differences either fail the gate or have an explicit documented platform reason.
- Public DTO and CLI JSON changes have consumer compatibility evidence and a declared migration path.
- Architectural recommendations preserve exact source evidence, confidence, scope, and unresolved alternatives instead of converting heuristics into facts.
