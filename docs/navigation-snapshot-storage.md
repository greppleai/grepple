# Navigation snapshot storage: Phase 3 investigation

Status: measured ReadAt detail-reader prototype; persistent hot sections and
production integration are not implemented. Existing protobufs and production
services are unchanged. No automatic artifact removal or cache flushing.

## Evidence and decision

The current resolved cache (`internal/navigation/navigation_resolved_cache.go`,
`readResolvedGraphCache`) reads and decodes the complete protobuf. Backend
`internal/navigation/adapter/content.go` likewise reads entire blobs; a
selective declaration decoder saves decoded facts but not the input blob read.
The experimental `querySnapshot` already traverses numeric adjacency, but
`snapshotDetailTable.blocks` retains every encoded detail block in Go memory.

Prefer a **flat, immutable, sectioned snapshot with explicit offsets**, first
accessed using `ReadAt`. A database is not warranted by these access patterns.
Do not select mmap until cold faults, resident memory and concurrent-generation
lifetimes are compared. Neither mmap nor moving bytes to files guarantees lower
RSS: file-backed pages still consume memory and can remain cached by the kernel.

Keep parsing, resolution, confidence and candidate order unchanged. The existing
protobuf remains a transition/rebuild input, not the normal query-start path.

## Measured first experiment

`snapshot_detail_readat.go` implements a private block-reader boundary. It checks
reference bounds before allocation, reads exactly one requested encoded block,
checks its SHA-256, then uses the existing bounded compression/native fact
validation. The source holds offsets/digests and a generation-owned ReaderAt,
not the full detail payload. Generation/file lifetime belongs to its caller.

Tests move already-built detail blocks into a temporary flat file and replace
resident byte slices with this reader. This proves lazy **detail** retrieval,
not a finished on-disk snapshot or direct reload of the hot graph.

Same preserved artifact: 55,370 declarations and 591,422 calls. Isolated process,
GOMAXPROCS=2, GOMEMLIMIT=384MiB, retained heap after test-only GC:

| Representation | Retained Go heap |
| --- | ---: |
| Rich graph reference | 440,804,288 bytes |
| Compact snapshot with resident encoded details | 221,482,840 bytes |
| Compact snapshot with ReadAt details | 88,944,480 bytes |

ReadAt details reduce retained heap ~80% versus the rich graph, and ~60% versus
the resident-detail compact version. `details.bin` contains 124,938,169 bytes;
this excludes persisted hot sections, dictionaries and the future manifest.
Do not mistake this for total format size or a production memory-budget result.

On a separate 10,000-node fixture, a depth-2 query performs **two block reads,
1,082 bytes total**, out of 751,670 detail bytes. Opening the reader performs
zero payload reads. Rich-fact differential tests, all five directions on the
large artifact, parallel readers, checksum/bounds failures, truncation and
pinned-open-file behavior pass. No cross-request decoded-detail cache is used.

One 200-sample warm synthetic run: ReadAt p50/p95 14.569/31.746 microseconds;
resident details 12.227/50.551 microseconds; reference 2.363/5.967 milliseconds.
Small-sample/order/GC noise means this does not show ReadAt is faster than RAM.
It establishes a plausible warm-I/O baseline, not cold-disk p95 acceptance.

Maximum process RSS was 800,980 KiB: this experiment still decodes the old graph,
converts it, then writes details. **The conversion peak is not solved.** OS cache
was not evicted; cold faults and post-idle/cgroup memory remain unmeasured.
Logs and profile: `/workspace/.tmp/nav-storage-measure`.

## Proposed query-ready layout (not a committed binary ABI)

Use one immutable data file plus a small publication manifest. A fixed header
and section directory identify independently addressable sections:

- Repository/ref/commit, source digest, resolver/config/executable fingerprint,
  schema, parse completeness and recovery information.
- Stable-ID/name/file/type lookup dictionaries; local numeric IDs remain local.
- Fixed-width callable/type/call records, candidate spans, duplicate identities.
- Outgoing and incoming CSR offsets and edge references.
- Owner/file indexes for type declarations, fields, usages, imports, exports and
  member accesses required by current query/architecture projections.
- Detail block index and payloads, preserving every rich semantic fact.

Encode fields explicitly, little-endian; never dump Go structs or persist map
iteration order. Store byte offsets as checked uint64 values, and validate
counts, multiplication, offsets, lengths, string spans and ID capacities. Keep
source ordering separate from identity lookup sorting. Preserve nil/empty output
shape and duplicate occurrences where compatibility requires them.

Avoid compressing an entire hot section as one stream: that would force full
section decompression. Detail blocks can remain independently compressed. Their
current 16-row granularity is a measured baseline, not a permanent optimum;
compare file-local/byte-sized blocks for rich and high-fanout corpora.

Manifest/header and index metadata have checksums; immutable pages/detail blocks
have individual digests. Validate only touched payloads on query open/read,
without hashing the whole data file on every request. Full validation belongs
at publication/offline verification. Hash metadata itself must be bounded and
validated, not treated as trustworthy because the payload has a checksum.

## Retrieve as little as possible

Opening a generation should read a bounded manifest/section directory, not the
protobuf or all rich facts. Then:

1. Resolve stable root IDs through indexed lookup (source/commit fenced).
2. Read CSR offsets for frontier nodes and only their edge-reference spans.
3. Read referenced fixed-width call records and needed candidate spans.
4. Fetch context rows through owner/file indexes, not whole-table scans.
5. Group selected detail rows by block and hydrate each touched block once.
6. Read original source only for selected snippets requiring it.

The first prototype keeps the compact core resident (~89 MB on this artifact).
That is already much smaller but still scales with repository size. The next
accessor layer should compare eager compact arrays with **paged ReadAt** arrays:
retain small offset/lookup indexes as justified by measurements, lazily fetch
calls/edges/candidates, and impose a process-wide byte-bounded page cache rather
than an unbounded cache per repository. Initially prefer request-local pages;
add shared retention only if measured latency needs it.

Large legitimate neighborhoods can still produce large output. Stream large
spans in bounded chunks; do not silently truncate candidate sets or infer edges
to satisfy a memory limit. Cancellation/resource limits must fail explicitly.

## Generation ownership, publication and recovery

Build into a new generation, streaming detail blocks to disk during freeze.
Validate all sections and semantic/source completeness; sync data and required
metadata before atomic manifest publication. Errors leave the prior manifest
and generation usable. Do not overwrite/truncate a generation used by readers.

A query pins one generation through traversal **and** hydration; its opened file
handle and block index must agree. Replacement readers use the new manifest,
while in-flight readers finish on the old generation. Closing/eviction waits for
leases; shared page keys include generation and section, not just repository.

Missing/corrupt/incompatible snapshots produce an explicit unavailable/rebuild
state. Rebuild using preserved parser facts/protobuf/source under a serialized,
resource-bounded background build policy. Do not hide full reconstruction inside
a bounded search request. Build **all outdated indexes**, independently of which
files are selected by a query. Do not bypass revision or dependency qualification.

Keep existing/historical protobufs and rollback generations during rollout.
Automatic old-generation deletion is disabled here; a future approved retention
policy must distinguish owned disposable generations from protected history.
No Zoekt file publication/lifecycle changes.

## Implementation gates

1. **Done as experiment:** private disk detail boundary and selective-read,
   corruption, parity, concurrent-reader and retained-heap measurements.
2. **Next:** stream detail blocks during freeze, then persist fixed-width hot
   sections and lookup indexes; reopen them without materializing NavigationGraph.
3. Compare eager core vs paged ReadAt, per-query bytes/reads/allocations, cold/warm
   p50/p95, heap, RSS/page faults, total disk bytes, build and direct reload peaks.
   Do not use cache eviction on production containers to create cold measurements.
4. Introduce the generation lease/publication/recovery owner and tests before
   public API/backend integration. Preserve exact artifact/dependency evidence.
5. Integrate selected-file related navigation on owning shards; keep global-page
   selection first. Codex manifest parsing and proactive build scheduling remain
   separate prerequisites, not fixed by this reader experiment.
6. Keep default rollout off until full source/command/architecture parity and
   direct-reload memory/latency gates pass. Incremental re-resolution is later.
