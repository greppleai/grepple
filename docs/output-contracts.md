# Output and execution contracts

Grepple keeps complete machine output available while bounding agent-facing text. Output mode, paging, source acquisition, projection limits, and delivery spilling are independent controls. A complete JSON page can still describe an intentionally bounded source universe, and a spilled artifact is complete even though stdout contains only its descriptor.

## Command output modes

| Command | Default human mode | Machine mode | Completeness contract |
| --- | --- | --- | --- |
| `search` | Human source, counts, paths, outlines, or matching lines; source rows are anchored when eligible. Human output defaults to 16,384 bytes. | `--json` returns complete structural results for the selected page; `--json-matches` returns only matching-line projections. | `--skip` and `--limit` page **result files**. `--max-files` bounds matching result files. Remote pages are capped at 100 files. Metadata reports page and analysis completeness separately. |
| `graph resolve|callers|callees` | Resolve previews ambiguous declarations; callers and callees show bounded source-signature call trees without routine schema/count/ID headers. Incomplete or truncated analysis emits a warning and continuation when available. | `--json` retains the queried navigation facts, source accounting, roots and filters, optional signatures, IDs, and metadata. | `--max-files` bounds discovered source files before graph construction; human byte limits do not truncate JSON. |
| `grit` | Human findings default to 20 and 16,384 output bytes. | `--json` returns the complete retained structural response and diagnostics. | `--skip` and `--limit` page **findings**. Scanner, compiler, source, memory, candidate, step, finding, and time limits remain authoritative and are reported in metadata. |
| `hook` | Repository-relative `PATH:LINE:COLUMN [ID] severity: message` findings plus a summary. File-local rules default to staged, unstaged, and untracked Git files; a selected relational rule forces a complete repository snapshot. | `--json` emits `grepple-hook-v1` with mode, candidate file count, selected IDs, and findings. | `--all` scans all selected sources; repeated `--id` selects specific hooks. `--workers N` bounds concurrent file checks (1-4, default 4). `gritql-relational-v1` joins normalized binding keys across files and supports optional partition queries; incomplete scans are errors. Finding exit 1, clean exit 0. Local-only and read-only. |
| `architecture directory` | Bounded source-linked directory output groups relation kinds and evidence by source/target pair; `--relations` compresses each pair and `--mermaid` renders the grouped dependency projection. | `--json` returns the complete directory inventory, declaration facts, and relation evidence for the selected source universe. | `--max-files` bounds analyzed files; `--depth` and `--max-nodes` bound displayed directories. |
| `tree` | Directory-first hierarchy defaults to top-level entries (`--depth 1`). Local output includes descriptions and fresh area tags; directory tags are sorted unions from selected descendant files, including those beyond displayed depth. Stale/invalid metadata is marked, not trusted as area membership. | `--json` includes optional `areas`, `description`, `metadataStatus`, and `metadataIssues` on the root and entries. | Repeated local `--area NAME` filters by any matching current tag, `--kind` intersects the result, and `--depth N` expands the hierarchy. Remote indexed trees reject `--area` because they cannot verify tag freshness. |
| `verify` | Summary plus missing (`M`), stale (`S`), and invalid (`I`) directory records. | `--json` emits `grepple-directory-metadata-verification-v1`, including `missing`, `stale`, and `invalid` lists when populated. | Exits with status 1 when a selected directory lacks current valid metadata. |
| `init` | One `skip` or `write` record per selected directory. | Generated `grepple.yaml` files are the output contract. | Existing metadata is preserved unless `--force` is supplied. Generation uses the configured model. |

Global delivery spilling is not semantic truncation. Output larger than `--spill-threshold-bytes` is written to a mode-0600 content-addressed artifact and stdout receives a `grepple-artifact-v1` descriptor. `--no-spill` keeps the same complete output on stdout. Human `--max-output-bytes` caps remain active unless explicitly set to zero.

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

Every source-reading range uses the same inclusive, 1-based EOF policy. When the requested start exists but the end exceeds the file, Grepple clamps the end to the final line, returns the available source, emits an explicit EOF warning, and succeeds. When the requested start is beyond the final line, the whole range is outside the file and the command fails. Complete search JSON carries `lineRange`; JSON-matches carries `lineRanges`; remote raw JSON carries `lineRange` and `warnings`; plain remote reads carry the same classification in `X-Grepple-Line-Range-Outcome` and `X-Grepple-Line-Range-Warning` headers. Context statistics schema `grepple-context-stats-v7` counts these as `details.partialLineRangeMisses` and `details.fullLineRangeMisses`.

## Local and remote availability

| Command family | Local checkout | Indexed remote repository | Selection |
| --- | ---: | ---: | --- |
| `search`, `grit` | Yes, default | Yes | Add `--remote`; local and remote results are merged. `--local` disables remote execution. |
| `graph`, `boundaries`, `architecture` | Yes, default | Yes, one exact selector | Use `--repo OWNER/REPO[@REF]` for remote execution; local paths and an exact remote selector are mutually exclusive where applicable. |
| `extract`, `write`, `sources`, `anchors`, `artifacts`, `context`, `init`, `verify` | Yes | No | These commands are local-only and reject unsupported remote selectors. |
| `repos`, `refs`, `get`, `rules`, `login` | No source analysis | Yes | These commands operate on the configured remote service. |
| `tree` | Yes, default | Yes | A local path is the default; use `--repo OWNER/REPO[@REF]` for an indexed tree. Directory descriptions are available when the tree payload contains them. |
| `ask` | Yes | Yes when a typed tool receives an exact indexed selector | The answer and tool evidence identify the selected local or remote universe. |
| `languages`, `examples`, `help`, `version` | Not source-dependent | Not source-dependent | These inspect capabilities or documentation. |

Command-specific `--help` is authoritative at the attempted command. A remote transport failure, unsupported remote operation, absent indexed selector, or unavailable local language capability is reported as a diagnostic or error; it is not silently retried against a different source universe.

## Resolution outcomes and confidence

Graph resolution outcomes answer **what happened**:

- `resolved-local`: one repository declaration was selected;
- `ambiguous-local`: multiple repository declarations remain;
- `unresolved-local`: no declaration was selected and the evidence is not an explicit unresolved import, including a single unpromoted candidate;
- `expected-external`: an explicit import path has no repository-local target.

Confidence labels answer **why the resolver assigned its evidence strength**, for example `exact`, `import-resolved`, `context-resolved`, `unique-terminal`, or `candidate`. `candidate` is only a confidence label in the authoritative outcome model. Complete graph JSON reports deterministic outcome counts and rates separately from confidence counts, overall and by language. Legacy `resolved`, `ambiguous`, `unresolved`, `candidate`, and `ambiguityRate` fields remain for compatible consumers.