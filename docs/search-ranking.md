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

Parser-backed segments are built in source order. Before `--max-segments` is applied, Grepple:

1. emits source lines for every matching top-level declaration or matched container child;
2. collapses nonmatching children inside a matched container to one-line summaries;
3. includes nearby top-level structural summaries (within two siblings of a matched declaration);
4. adds one-line source segments for uncovered matching lines;
5. merges adjacent source ranges and removes a summary that starts where retained source already starts.

When the segment cap applies, matching source segments rank before summary/context segments. Each class retains source order, and the selected segments are sorted back into source order for rendering. If matching lines are omitted, human output reports the count and recommends `--line-only`; complete JSON publishes the configured segment cap in result metadata.

This policy prioritizes direct evidence over contextual summaries without pretending that a later declaration is less semantically relevant than an earlier one. Use `--line-only`, a narrower path, or a higher `--max-segments` when every occurrence matters.
