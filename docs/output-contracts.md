# Output and execution contracts

Grepple keeps complete machine output available while bounding agent-facing text. Output mode, paging, source acquisition, projection limits, and delivery spilling are independent controls. A complete JSON page can still describe an intentionally bounded source universe, and a spilled artifact is complete even though stdout contains only its descriptor.

## Command output modes

| Command | Default human mode | Machine mode | Completeness contract |
| --- | --- | --- | --- |
| `search` | Human source, counts, paths, outlines, or matching lines; source rows are anchored when eligible. Local human outlines show verified scope selectors when available. Human output defaults to 16,384 bytes. | `--json` returns complete structural results for the selected page without outline selector decoration; `--json-matches` returns only matching-line projections. | `--skip` and `--limit` page **result files**. `--max-files` bounds matching result files. Repeatable local metadata selectors narrow checksum-current source candidates **before** scanning/paging; no-match exits 1. Remote, cursor, stdin and explicit related navigation cannot combine with selectors. Remote pages are capped at 100 files. Metadata reports page and analysis completeness separately. |
| `graph resolve|callers|callees` | Resolve previews ambiguous declarations; callers and callees show bounded source-signature call trees without routine schema/count/ID headers. Incomplete or truncated analysis emits a warning and continuation when available. | `--json` retains the queried navigation facts, source accounting, roots and filters, optional signatures, IDs, and metadata. | `--max-files` bounds discovered source files before graph construction; human byte limits do not truncate JSON. |
| `grit` | Human findings default to 20 and 16,384 output bytes. | `--json` returns the complete retained structural response and diagnostics. | `--skip` and `--limit` page **findings**. Scanner, compiler, source, memory, candidate, step, finding, and time limits remain authoritative and are reported in metadata. |
| `hook` | Repository-relative `PATH:LINE:COLUMN [ID] severity: message` findings plus a summary. File-local rules default to staged, unstaged, and untracked Git files; a selected relational rule forces a complete repository snapshot. | `--json` emits `grepple-hook-v1` with mode, candidate file count, selected IDs, and findings. | `--all` scans all selected sources; repeated `--id` selects specific hooks. `--workers N` bounds concurrent file checks (1-4, default 4). `gritql-relational-v1` joins normalized binding keys across files and supports optional partition queries; incomplete scans are errors. Finding exit 1, clean exit 0. Local-only and read-only. |
| `architecture directory` | Bounded source-linked directory output groups relation kinds and evidence by source/target pair; `--relations` compresses each pair and `--mermaid` renders the grouped dependency projection. | `--json` returns the complete directory inventory, declaration facts, and relation evidence for the selected source universe. | `--max-files` bounds analyzed files; `--depth` and `--max-nodes` bound displayed directories. |
| `tree` | Directory-first hierarchy defaults to top-level entries (`--depth 1`). Local file and ancestor labels are derived from selected, checksum-current annotations; text shows up to two labels per category and `+N` for omitted labels. Stale/invalid metadata is marked, never trusted for filtered membership. | `--json` includes optional bounded `labels` (per-category examples and full totals), `description`, `metadataStatus`, and `metadataIssues` on local root and entries. Remote indexed trees retain their existing `areas` contract. | Repeated local `--purpose`, `--domain-area`, `--functional-area`, and `--keyword` values OR within each category and AND across categories on the same file before independent `--depth N` projection. All four reject remote indexes. A filtered no-match emits empty entries and exits 1 (text or JSON) with stderr guidance. |
| `verify` | Summary plus missing (`M`), stale (`S`), and invalid (`I`) directory records. | `--json` emits `grepple-directory-metadata-verification-v1`, including `missing`, `stale`, and `invalid` lists when populated. | Exits with status 1 when a selected directory lacks current valid metadata. |
| `overview` | Short local vocabulary, selected/current/stale/missing/invalid counts, and selector examples. | `--json` emits bounded `grepple-overview-v1` with complete counts and omitted-label totals. | Uses the same selected-file inventory as verify/tree; reads metadata and checksums but never calls a model. |
| `init` | One `skip` or `write` record per selected directory. | Generated entries in `.grepple/grepple.yaml` are the output contract. | Current selected annotations are skipped unless `--force` is supplied; stale/missing selected annotations regenerate, and checksum-current excluded siblings remain intact. Generation invokes the configured model only when needed. |

Global delivery spilling is not semantic truncation. Output larger than `--spill-threshold-bytes` is written to a mode-0600 content-addressed artifact and stdout receives a `grepple-artifact-v1` descriptor. `--no-spill` keeps the same complete output on stdout. Human `--max-output-bytes` caps remain active unless explicitly set to zero.

## Compact navigation evidence

Compact callers and callees show each distinct relationship evidence once per selected root section. Separate call IDs with the same caller, target, source path/line, confidence, ambiguity status, and direction share one compact row. Distinct locations, targets, confidence levels, and candidate relationships remain visible, including unique cycle/back edges; declaration expansion is cycle-safe and does not add an `(already shown)` suffix. Independent roots retain their own evidence. Duplicate rows are suppressed before consuming the human-output byte budget.

This is presentation-only deduplication, shared by local, daemon, and remote rendering. Graph JSON retains individual call-site IDs and facts, including separate calls on the same source line. Parser facts and the graph schema are unchanged; compact output is not a lossless enumeration of call occurrences.

## Limit units

| Option or concept | Unit |
| --- | --- |
| Search `--skip`, `--limit` | ranked result files |
| Search `--max-files` | matching result files admitted before paging |
| Graph, boundary, architecture, and GritQL `--max-files` | discovered or eligible source files acquired for analysis |
| GritQL `--skip`, `--limit`, `--max-findings` | structural findings |
| Boundary `--limit` | candidates in each human-output section |
| Extract/architecture `--max-nodes` | Mermaid or compact directory nodes |
| `--depth` | traversal hops or displayed hierarchy levels, as named by the command help |
| `--max-output-bytes`, spill thresholds | encoded output bytes |
| GritQL source/pattern/regex/memory/total-byte limits | bytes |
| GritQL `--max-candidates` | structural candidates per file |
| GritQL `--max-ast-steps` | charged structural evaluation steps per file |
| GritQL file/batch time limits | milliseconds |

Zero means unlimited only where the command help explicitly says so. Remote services may impose stricter page or resource ceilings; metadata and diagnostics disclose the effective result rather than presenting a partial page as complete.

## Line-range bounds

Legacy source-reading `PATH:START-END` ranges use inclusive, 1-based EOF handling: when the requested start exists but the end exceeds the file, Grepple clamps the end to the final line, returns available source, emits an explicit EOF warning, and succeeds. A start beyond EOF fails. In contrast, local parser-verified `--at PATH @KIND:START-END` must match **both** current scope endpoints and never clamps; `@KIND:LINE` requires an unambiguous verified containing scope. Such selector requests fail without source output on stale/missing/uncertain bounds. Complete search JSON carries `lineRange`; JSON-matches carries `lineRanges`; remote raw JSON carries `lineRange` and `warnings`; plain remote reads carry the same classification in `X-Grepple-Line-Range-Outcome` and `X-Grepple-Line-Range-Warning` headers for legacy reads. Each repeat of a valid legacy range returns the selected available source with the same warning; no context statistics or cache is maintained. Decorated `--line-only` search rows are not edit-ready; typed JSON keeps literal `text` separate from `nearest`/`scope`, and a verified local selector returns clean literal source.

## Local and remote availability

| Command family | Local checkout | Indexed remote repository | Selection |
| --- | ---: | ---: | --- |
| `search`, `grit` | Yes, default | Yes | Add `--remote`; local and remote results are merged. `--local` disables remote execution. |
| `graph`, `architecture` | Yes, default | Yes, one exact selector | Use `--repo OWNER/REPO[@REF]` for remote execution; local paths and an exact remote selector are mutually exclusive where applicable. |
| `ask` | Yes, default | Yes, via typed tools with exact repository selectors | Authenticate an AI provider; `--server URL` selects a remote Grepple service but does not select a repository on its own. |
| `write`, `sources`, `anchors`, `artifacts`, `init`, `verify` | Yes | No | These commands are local-only and reject unsupported remote selectors. |
| `repos`, `refs`, `get`, `rules`, `login` | No source analysis | Yes | These commands operate on the configured remote service. |
| `tree` | Yes, default | Yes | A local path is the default; use `--repo OWNER/REPO[@REF]` for an indexed tree. Directory descriptions are available when the tree payload contains them. |
| `languages`, `examples`, `help`, `version` | Not source-dependent | Not source-dependent | These inspect capabilities or documentation. |

Command-specific `--help` is authoritative at the attempted command. A remote transport failure, unsupported remote operation, absent indexed selector, or unavailable local language capability is reported as a diagnostic or error; it is not silently retried against a different source universe.

## Resolution outcomes and confidence

Graph resolution outcomes answer **what happened**:

- `resolved-local`: one repository declaration was selected;
- `ambiguous-local`: multiple repository declarations remain;
- `unresolved-local`: no declaration was selected and the evidence is not an explicit unresolved import, including a single unpromoted candidate;
- `expected-external`: an explicit import path has no repository-local target.

Confidence labels answer **why the resolver assigned its evidence strength**, for example `exact`, `import-resolved`, `context-resolved`, `unique-terminal`, or `candidate`. `candidate` is only a confidence label in the authoritative outcome model. Complete graph JSON reports deterministic outcome counts and rates separately from confidence counts, overall and by language. Legacy `resolved`, `ambiguous`, `unresolved`, `candidate`, and `ambiguityRate` fields remain for compatible consumers.