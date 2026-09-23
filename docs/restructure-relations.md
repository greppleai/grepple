# Restructure relation review

This review compares the pre-restructure dependency evidence recorded in `restructure.md` with the production architecture generated after the migration. Parser recovery and unresolved relations remain completeness qualifications.

## Removed reverse and lateral edges

- `analysis -> api` was replaced by Analysis-owned queries and reports.
- `search -> api`, `navigation -> api`, and `rulespec -> api` were reversed: API now aliases canonical domain models.
- The former `internal/sourcepolicy`, `internal/sourcecatalog`, `internal/sourceinspection`, and `internal/sourcekind` packages were consolidated into `internal/sources`; engine projections still point inward from Search and Extract, and Directory Metadata remains the persisted representation owner.
- `internal/cli/ask -> internal/cli/anchors`, `internal/cli/search -> internal/cli/anchors`, and other sibling-command dependencies were replaced by neutral packages or Analysis APIs.
- `cmd/grepple -> parser` was removed; process defaults are applied at the CLI composition boundary.
- Boundary heuristics moved out of Search into the Analysis-owned boundary implementation.

## Intentional new edges

- `api -> search|navigation|rulespec`: source- and JSON-compatible transport aliases point to canonical domain models.
- `analysis -> internal/boundaryanalysis`: Analysis exposes the reusable report facade while the cohesive heuristic engine remains internal.
- `search -> internal/boundaryanalysis`: exported Search boundary names remain compatibility aliases until a breaking release; no heuristic implementation remains in Search.
- `internal/cli/graph -> navigation` and `internal/cli/boundaries -> analysis|navigation`: command adapters now call canonical owners instead of Search forwarding wrappers.
- `internal/render -> search|navigation`: renderers consume canonical result and relationship models. Its remaining API edge is limited to transport envelopes, metadata, source summaries, and Tree responses.
- `api -> rulespec -> search`: saved rules embed the canonical Search request model; dependency flow remains inward and acyclic.

## Containment conclusion

The production graph remains a DAG with Parser foundational, Navigation consuming Parser facts, Search and Analysis consuming Navigation capabilities, and CLI packages terminating dependency flow. New edges either reverse former transport leakage or connect command adapters to the canonical implementation owner. No new sibling-command import, core-to-CLI import, Parser-to-Navigation, source-policy-to-engine, or domain-to-API edge is accepted. Two directory relations that resemble lateral edges are qualified navigation artifacts rather than imports: `internal/cli/ask -> internal/cli/sources` resolves a local callback named `run`, and `internal/outputspill -> internal/cli/sources` resolves a function parameter named `run`; exact source inspection shows no package dependency, and the import-policy test rejects any actual sibling-command import.