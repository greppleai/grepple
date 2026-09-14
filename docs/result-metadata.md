# Result metadata

Complete JSON responses from local text search, navigation graph operations, boundary analysis, and CLI GritQL include the same bounded-execution envelope. Search uses `metadata`; GritQL uses `resultMetadata` because `metadata` already names its grammar/compatibility contract.

```json
{
  "scope": {
    "mode": "local",
    "paths": ["."],
    "repositories": [],
    "languages": ["go"]
  },
  "page": {
    "skip": 0,
    "limit": 20,
    "returned": 20,
    "complete": false
  },
  "limits": {
    "maxFiles": 0,
    "maxSegments": 20,
    "maxOutputBytes": 16384,
    "maxSourceBytes": 0,
    "maxTotalBytes": 0,
    "jsonByteUncapped": true
  },
  "omitted": {
    "files": 0,
    "segments": 0,
    "sources": 0,
    "findings": 0,
    "bytes": 0
  },
  "diagnostics": [],
  "nextCommand": "grepple search --skip 20 --limit 20 --json query ."
}
```

## Interpretation

- `scope` is the normalized selected universe, not a claim about files outside it.
- `page.total` is present only when the operation can prove the total without estimating. `page.complete=false` plus an absent total means the remainder is unknown.
- Zero limits mean unlimited for that local dimension. `maxOutputBytes` describes human output; JSON remains byte-uncapped when `jsonByteUncapped=true`.
- Omission fields contain known counts only. Unknown omissions remain zero and are disclosed through `page.complete`, truncation records, or diagnostics rather than guessed.
- Domain-specific source statistics, GritQL statistics/truncations, and boundary candidate counts remain authoritative details.
- `diagnostics` provides stable cross-command codes while preserving richer domain diagnostics alongside it.
- `nextCommand`, when present, is shell-quoted and copyable. Paging continuations preserve scope; source-cap continuations explicitly remove the relevant source cap.

Human output retains its task-specific compact presentation, but bounded graph, boundary, search, and GritQL paths print copyable continuation commands whenever their metadata identifies an incomplete next step.
