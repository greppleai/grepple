# Repository-local memories

This example is a template for **your own repository**, not for the checked-in sample fixtures. See the full [format and matching rules](../docs/repository-memories.md). First prepare `notes.md` with a private, nonempty UTF-8 Markdown note. The repository must have a `.git` or `.grepple` marker. A draft `notes.md` outside `.grepple/memories/` is an ordinary source file: keep it out of source selection or remove it if its contents should not appear in ordinary search.

```sh
grepple --outline internal/parser/analyze.go  # inspect current selectors
# Replace paths and line numbers below with your own selected source and its actual match lines.
# Replace paths and line numbers below with your own selected source and its actual match lines. Quote glob scopes so the shell cannot expand them.
grepple memory store parser-note --description 'Review parser call sites' \
  --body-file ./notes.md --scope internal/parser/ --scope 'internal/parser/**/*.go' \
  --line-range internal/parser/analyze.go:100-300 \
  --domain-area source-parsing
grepple --line-only -F 'Parse' internal/parser/analyze.go  # short reference only
grepple memory recall parser-note                # full body (stdout), attachment statuses (stderr)
# After edits, explicitly update ALL fields, including current attachment and complete body.
# `update` replaces every prior field: repeat all desired scopes, labels, files, and body.
grepple memory update parser-note --description 'Reviewed parser note' \
  --body-file ./revised.md --scope internal/parser/ --scope 'internal/parser/**/*.go' \
  --line-range internal/parser/analyze.go:100-300 \
  --domain-area source-parsing
grepple memory delete parser-note
```

A stale checksum stops attachment matching; line/symbol bounds are never silently rebased. A scope (`directory/`, `directory/*.go`, or `directory/**/*.go`) is path relevance for already selected local result paths, not verified current source bytes and not an edit-time enforcement policy. It can cover future selected files without expanding a glob or changing source selection. The search footer is limited to three safe references, with an incomplete indicator if inspection fails. JSON includes optional `memories` and `memoryStatus` fields. This is local-only: no footer for remote/mixed or stdin, files-only, counts, outlines, explicit anchors or explicit related navigation. The full body, paths, checksums, and scopes are never included in an implicit search response.
