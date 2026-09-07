# Counting & listing probes

Before dumping file bodies, use cheap bounded probes to see *where* a term concentrates
and *which* files match. These skip tree-sitter parsing and return far fewer tokens.

## Count (`-c` / `--count`)

A per-directory tally of files and matches, hottest first, ending in a `total` row:

```bash
grepple -c "grepple" sample-files
```

```text
sample-files/deployment.yaml	1 files	5 matches
sample-files/values.yaml	1 files	4 matches
sample-files/config.json	1 files	1 matches
total	3 files	10 matches
```

Count composes with a glob, so you can scope it to one file type:

```bash
grepple -F "app" "sample-files/*.yaml" --count
```

```text
sample-files/deployment.yaml	1 files	4 matches
total	1 files	4 matches
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
