# Parser syntax lifecycle

Grepple exposes three syntax access modes over one parser-owned `Document`. They trade retention and allocation for traversal cost; they do not represent three independent parsers.

| Surface | Ownership and lifetime | Locking | Retain after callback or `Close`? | Intended use |
| --- | --- | --- | --- | --- |
| `Document` | Owns an immutable source copy and one syntax tree until `Close` | Methods coordinate with `Close` | The document may be retained, but becomes closed | Shared parse/cache boundary and high-level analysis input |
| `Node` | Borrowed handle into one `Document` | Every accessor takes the document read lock | After `Close`, accessors return invalid/zero results | Occasional navigation when callback-scoped traversal is inconvenient |
| `DocumentView` | Borrowed view valid only during `Document.Read` | `Read` holds one read lock for the whole callback | No | Coherent, allocation-light traversal |
| `ViewNode` | Borrowed node tied to its active `DocumentView` | Accessors are lock-free inside the callback | No; `Valid` becomes false after return | Repeated tree walking and field inspection |
| `SyntaxNode` | Deep immutable snapshot of a node subtree and source ranges | No document lock after construction | Yes | Persisting syntax evidence beyond a read callback or document lifetime |

## Document ownership

`ParseDocument` validates UTF-8, clones the source, parses through the owning `languageAdapter`, and returns a `Document`. The caller should call `Close`; the finalizer is only a safety net. `Close` is idempotent and may run concurrently with ordinary `Document` or `Node` reads. A `Document` contains synchronization state and must not be copied after first use.

High-level consumers should prefer document-backed APIs such as `OutlineFromDocument`, `NavigationGraphFromDocument`, and `CachedNavigationGraphFromDocument`. These derive facts from the same tree without reparsing and leave ownership with the caller.

## Stable callback traversal

Use `Document.Read` when several node operations must observe one coherent open tree:

```go
err := document.Read(func(view parser.DocumentView) error {
    parser.WalkNamedView(view.Root(), func(node parser.ViewNode) {
        // Inspect node.Kind(), node.Range(), fields, and children here.
    })
    return nil
})
```

The callback must use only its `DocumentView` and `ViewNode` values. It must not call methods on the owning `Document`, wait for code that may close the document, or retain view values after returning. `ViewNode` accessors avoid per-operation document locking because `Document.Read` already holds the read lock; the primary contract is coherent traversal, not a promise that every workload is faster than `Node`.

## Retaining syntax safely

Call `Node.Snapshot` or `ViewNode.Snapshot` when syntax must outlive its borrowed handle. `SyntaxNode` recursively copies node kind, text, field, flags, ranges, children, and a reference to the immutable source string. It remains valid after `Document.Close` and is safe for concurrent reads. Snapshotting a large subtree allocates proportionally to that subtree, so snapshot the narrowest useful node.

A retained `Node` is not a snapshot. It remains tied to its document and becomes invalid when the document closes. A sequence of separate `Node` calls is individually safe but does not reserve the tree across the whole sequence; use `Document.Read` for that guarantee.

## Invalid and recovery behavior

Zero `Node`, `ViewNode`, and `SyntaxNode` values are invalid. Borrowed-node accessors return zero values after invalidation instead of dereferencing a released tree. `ParseDocument` can return a valid recovery tree; inspect `ParseDiagnostics` or `Root().HasError()` when completeness matters.

All ranges are half-open. Byte offsets are zero-based; line and Unicode-scalar columns are one-based. Grammar node kinds and field names remain language-specific even though the handles are language-neutral.

## Measured tradeoff

`BenchmarkSyntaxAccessModes` tests whether these surfaces are merely ownership aliases. On the reviewed 100-function Go fixture (Linux/amd64, Intel Core Ultra 7 165H, Go 1.25.14, `-benchtime=20x`), document-tied `Node` traversal measured 3.88 ms/0.40 MB, callback-scoped `ViewNode` traversal 5.18 ms/0.64 MB, retained snapshot traversal 0.32 ms/0.62 MB, and snapshot creation 4.48 ms/0.84 MB. Timing is machine-dependent. The result does not justify choosing a view solely for speed: `Document.Read` exists for one coherent lock scope, `Node` for retainable document-tied convenience, and `SyntaxNode` for post-close ownership. Snapshot creation has an explicit up-front cost. These distinct safety and retention contracts currently justify retaining all three access modes.
