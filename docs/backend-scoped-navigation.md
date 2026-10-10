# Backend adoption: language-connected paged generations

Backend source now publishes paged language-connected partitions through the
public API navigation capability. No public-to-backend import, alternate resolver,
or deployment is involved. Running shards and installed CLI remain unchanged.

The existing graph.pb and query.gnav remain. Each new publication writes immutable
scoped-<unique>/N.gnav files first, then atomically publishes the query.gnav root
with partition paths/keys/languages in its checked metadata. That root commit
covers one repository/commit and one immutable partition directory. Unsafe paths,
symlinks and mixed directories fail validation. Historical generations are not
removed. This currently **adds storage**, not a disk-saving deployment.

Selected result languages pick one complete connected group when possible.
Multiple independent groups use the previous complete snapshot path rather than
partial results. Missing/corrupt selected pages return errors; there is no rich
budget-error fallback. Declaration-only dependency resolution stays on the
existing root lease, and compatibility downloads retain their graph.pb provenance.
Production/custom-ignore portable paths and exact selected owner evidence remain
unchanged. Scope grouping delegates to the same public navigation engine as CLI.

GNSTO005 adds an independently checked selected-file-to-root locator to the prior
GNSTO004 numeric/name/key layout. Related traversal includes incoming root calls,
requested outgoing depth, unresolved calls and duplicate call groups. Declaration/
type lookup is still eager **within the connected partition**. It does not yet
load only a package/type context island, and large Rust groups still allocate.

## Migration correctness

Indexed Codex exposed a real conversion issue: graph.pb's compatibility round trip
can change empty-versus-nil candidate slices. Existing active query.gnav is the
cold migration source through explicit OpenArtifact, preserving native shapes.
Host revision/identity and compatibility-content digest are validated before new
publication. An existing corrupt active generation fails migration, rather than
silently rebuilding from a lossy protobuf. If no active snapshot exists, existing
compatibility import/native source construction remains available. Required missing
compatibility artifacts still rebuild through the established source path.

Migration/construction is serialized by the existing build mutex. It still occurs
on activation; proactive build-all-outdated scheduling and navigation-ready gating
are outstanding. No claim of eliminating first-request build work.

## Indexed Codex experiment (2026-10-09)

Read-only copies from the existing shard-b indexed artifact, commit
7784318b5f7fa35728d41ffa13e2a5821ebb4d75. No live volume/index/source mutation.
Eight connected language groups: C, C++, CSS, JavaScript, Python, Rust, shell,
TypeScript. Rust: 53152 declarations and 574406 calls. Selected file
openai/codex/codex-rs/core/src/exec.rs, related depth 2: 1778 selected calls.

After accounting for intentionally excluded unrelated-language lookup context,
projection facts compare exactly, including candidate shapes/order and duplicates.
The resulting enriched public result also compares exactly using copied selected
source content. Other preview source files are unavailable in the isolated fixture;
this is not a live HTTP/dependency-owner end-to-end test. A discovered empty/stale
optional-preview rendering panic was fixed: retain the fact/coordinates, omit
unavailable source segments instead of slicing past missing content.

Five serial reopen/projection samples, GOMAXPROCS=1/GOMEMLIMIT=384MiB, ordinary warm
filesystem behavior. Measurement GC is test-only. Same binary and indexed facts:

| Metric | Previous complete snapshot | Paged Rust partition |
| --- | ---: | ---: |
| Median reopen + related projection | 983.80 ms | 452.94 ms |
| Mean allocated bytes | 391545149 | 245884384 |
| Separate-process peak RSS | 247116 KiB | 106664 KiB |

Approximately 2.17x reader speed, 37% fewer allocated bytes. RSS is not retained
heap, anonymous cgroup memory, or proof of no leaks. These are not HTTP latency or
cold-cache numbers. Rust paged artifact: 207078348 B. Active-snapshot conversion
and publication peaked at 1162084 KiB RSS in this isolated test: cold conversion
still matters and fresh parsing/building has not been remeasured here.

Artifacts: /workspace/.tmp/backend-scoped-codex, including preserved original
copies, identity/control files, build-active.{log,time}, parity-active.log,
legacy-active.{log,time}, paged-active.{log,time}, rust-paged.gnav and api.test.
Pre-fix build/measurement logs remain historical, not accepted parity results.

## Remaining work before production rollout

- Context-aware admission, actual byte reservations for directories, native detail
  expansion/returned context/output, and build/query coexistence. The shared reader
  currently has two traversal slots and explicit read/selection budgets, not a
  hard total-cgroup reservation; leases/directory opening happen outside it.
- Proactive independent outdated-generation builds and navigation readiness.
- Repeated indexed Python/small Rust/large Rust/Codex multi-page queries, concurrent
  request/build/restart/fault tests, physical read/detail bytes and post-idle cgroup
  anonymous memory. Live response/private authorization parity before deployment.
- Selective native declaration/type contexts and full multi-group traversal instead
  of compatibility fallback; stream cold construction and bound build scratch.

Backend deployment still requires operation-specific approval. No commit or push.
