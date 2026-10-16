# Counting & listing probes

Before dumping file bodies, use cheap bounded probes to see *where* a term concentrates
and *which* files match. These skip tree-sitter parsing and return far fewer tokens.

## Count (`-c` / `--count`)

`-c` reports matching-line counts per file in path order (not a directory summary):

```bash
grepple -c "grepple" sample-files
```

```text
sample-files/config.json	1
sample-files/deployment.yaml	5
sample-files/values.yaml	4
```

For a complete, paging-independent total, use `--count-summary` instead:

```bash
grepple --count-summary "grepple" sample-files
```

```text
3 files	10 matches
```

Count also composes with a glob, so you can scope it to one file type:

```bash
grepple -F "app" "sample-files/*.yaml" --count
```

```text
sample-files/deployment.yaml	6
```

## List files whose contents match (`--files-with-matches`)

Like `grep -l` — paths only, no bodies:

```bash
grepple "8080" --files-with-matches sample-files
```

```text
sample-files/config.json
sample-files/deployment.yaml
sample-files/server.log
sample-files/values.yaml
```

## List files by name (`-l` / `--files`)

`-l` takes a **glob matched against the path** (not a content pattern). `*` does not
cross `/`, so use `**` to descend directories:

```bash
grepple -l "sample-files/*.go"
```

```text
sample-files/server.go
```

### Scope a glob to a directory

A bare directory positional acts as a **scope root**, so a filtering glob can be
narrowed to one subtree. On its own the glob matches everywhere:

```bash
grepple -l "**/*.tsx"
```

```text
advanced-files/SearchPanel.tsx
sample-files/Button.tsx
sample-files/pkg/widget.tsx
```

Add a directory and the same glob is scoped to just that subtree (the directory
narrows *where* to look; the glob still decides *what* to keep):

```bash
grepple -l "**/*.tsx" sample-files/pkg
```

```text
sample-files/pkg/widget.tsx
```
