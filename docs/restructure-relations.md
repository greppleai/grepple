# Restructure relation review

This review compares the pre-restructure dependency evidence recorded in `restructure.md` with the production architecture generated after the migration. Parser recovery and unresolved relations remain completeness qualifications.

## Removed reverse and lateral edges

- `analysis -> api` was replaced by Analysis-owned queries and reports.
- `search -> api`, `navigation -> api`, and `rulespec -> api` were reversed: API now aliases canonical domain models.
- The former `internal/sourcepolicy`, `internal/sourcecatalog`, `internal/sourceinspection`, and `internal/sourcekind` packages were consolidated into `internal/sources`; engine projections still point inward from Search and Extract, and Directory Metadata remains the persisted representation owner.
- Former `internal/cli/ask -> internal/cli/anchors`, `internal/cli/search -> internal/cli/anchors`, and other sibling-command dependencies were replaced by neutral packages or Analysis APIs. The restored `grepple ask` command consumes shared source and transport services instead of importing sibling commands.
- `cmd/grepple -> parser` was removed; process defaults are applied at the CLI composition boundary.
- Boundary heuristics moved out of Search into the Analysis-owned boundary implementation.

## Intentional new edges

- `api -> internal/wire -> internal/search|internal/navigation|internal/rulespec`: the facade re-exports wire-compatible transport contracts; internal adapters consume wire contracts directly, never `api`.
- `analysis -> internal/boundaryanalysis`: Analysis exposes the reusable report facade while the cohesive heuristic engine remains internal.
- `search -> internal/boundaryanalysis`: exported Search boundary names remain compatibility aliases until a breaking release; no heuristic implementation remains in Search.
- `internal/cli/graph -> internal/navigation`: the focused graph command calls the canonical navigation owner. The removed `internal/cli/boundaries` adapter has no incoming command edge; backend boundary analysis continues through `api -> internal/analysis -> internal/boundaryanalysis`.
- `internal/render -> internal/search|internal/navigation|internal/wire`: renderers consume canonical result and relationship models. Their wire edge is limited to transport envelopes, metadata, source summaries, and Tree responses.
- `internal/wire -> internal/rulespec -> internal/search`: saved rules embed the canonical Search request model; dependency flow remains inward and acyclic.

## Containment conclusion

The production graph remains a DAG with Parser foundational, Navigation consuming Parser facts, Search and Analysis consuming Navigation capabilities, and CLI packages terminating dependency flow. New edges either reverse former transport leakage or connect command adapters to the canonical implementation owner. No new sibling-command import, core-to-CLI import, Parser-to-Navigation, source-policy-to-engine, or domain-to-API edge is accepted. Two directory relations that resemble lateral edges are qualified navigation artifacts rather than imports: `internal/cli/ask -> internal/cli/sources` resolves a local callback named `run`, and `internal/outputspill -> internal/cli/sources` resolves a function parameter named `run`; exact source inspection shows no package dependency, and the import-policy test rejects any actual sibling-command import.