# Search and segment ranking

Grepple has two separate deterministic ordering decisions: which matching files form a page, and which structural segments represent one returned file.

## File order

`--sort path` is the default. Files are ordered by repository and normalized display path. A bounded local search can stop reading once the requested path-ordered window is full, which keeps broad indexed searches cheap and makes repeated paging stable.

`--sort matches` is an explicit, deterministic breadth heuristic:

1. matching-line count descending;
2. repository ascending;
3. normalized display path ascending.

```bash
grepple search --sort matches --limit 20 -F 'Register(' .
```

Match-count ordering must scan every candidate in the selected universe before applying `--skip`/`--limit`; it trades acquisition cost for an explainable broad-result ordering. It is not semantic relevance: it does not use embeddings, source recency, file size, language preference, symbol popularity, or nondeterministic index scores. Narrow paths and queries remain preferable when the target is known.

The strategy was deliberately limited to matching-line count after evaluating more opaque alternatives. Fuzzy similarity and weighted syntax scores would make results harder to predict and compare, while recency and index-native scores would vary across checkouts or reindexes. The count/path rule is stable locally and after local/remote result merging.

## Segment selection inside a file

Parser-backed segments are built in source order. Grepple:

1. emits the complete top-level function, method, or declaration containing each direct match;
2. for class, trait, impl, namespace, and similar containers, retains the owner wrapper and only children containing direct matches;
3. keeps function-like scopes complete, including JSX/TSX functions, rather than compacting them to matching statements;
4. adds one-line source segments only for matches not covered by a parsed declaration;
5. merges adjacent retained source ranges.


This policy prioritizes complete matching callables without injecting nearby imports, top-level declarations, or nonmatching sibling summaries. Use `--line-only`, bounded context, or a narrower path when less source is wanted.
