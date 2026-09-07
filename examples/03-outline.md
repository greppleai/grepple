# Outlines (`--outline` / `-O`)

`--outline` prints a file's structure instead of searching it. Output is
`START-END<TAB>KIND<TAB>NAME`, with nested members indented under their container.
It is a cheap way to orient in an unfamiliar or large file.

## Go symbols

```bash
grepple -O sample-files/server.go
```

```text
sample-files/server.go	go
9-12	struct	Config
15-18	struct	Server
21-23	func	NewServer
26-28	method	(*Server).Handle
31-38	method	(*Server).ListenAndServe
```

Go methods appear at top level as `(*Type).Method`, in source order.

## TSX symbols

```bash
grepple -O sample-files/Button.tsx
```

```text
sample-files/Button.tsx	tsx
3-6	type	ButtonProps
8-21	function	Button
```

## Java symbols

Classes nest their members (records, fields, constructors, methods) with line
ranges:

```bash
grepple -O sample-files/Order.java
```

```text
sample-files/Order.java	java
5-25	class	Order
  6-6	record	LineItem
  8-8	field	id
  9-9	field	items
  11-14	constructor	Order
  16-20	method	total
  22-24	method	describe
```

## Kotlin symbols

Top-level `data class` and `class` declarations, with member functions nested
under their class:

```bash
grepple -O sample-files/Inventory.kt
```

```text
sample-files/Inventory.kt	kotlin
3-3	class	Product
5-19	class	Inventory
  6-7	fun	find
  9-15	fun	restock
  17-18	fun	lowStock
```

## JSON key/type tree

For JSON and YAML the outline is a key skeleton with **values omitted** — only keys and
types. Arrays show their length as `[N]`.

```bash
grepple -O sample-files/config.json
```

```text
sample-files/config.json	json
2-2	string	name
3-3	string	version
4-9	object	server
  5-5	string	host
  6-6	number	port
  7-9	object	tls
    8-8	bool	enabled
    9-9	string	minVersion
12-12	array	features [3]
13-15	object	limits
  14-14	number	maxFiles
  15-15	number	timeoutSeconds
```

### Cap nesting with `--depth`

`--depth N` limits how deep the JSON/YAML tree recurses (top level is depth 1):

```bash
grepple -O --depth 1 sample-files/config.json
```

```text
sample-files/config.json	json
2-2	string	name
3-3	string	version
4-9	object	server
12-12	array	features [3]
13-15	object	limits
```

## YAML key/type tree

The same key/type skeleton works for YAML, including nested arrays of objects.
This pays off on larger config files where the shape is the win, not the payload:

```bash
grepple -O sample-files/values.yaml
```

```text
sample-files/values.yaml	yaml
3-9	object	image
  4-4	string	repository
  5-5	string	tag
  6-6	string	pullPolicy
  7-9	array	pullSecrets [2]
    8-8	object	[0]
      8-8	string	name
    9-9	object	[1]
      9-9	string	name
10-16	object	service
  11-11	string	type
  12-12	number	port
  13-16	object	annotations
    14-14	string	prometheus.io/scrape
    15-15	string	prometheus.io/path
    16-16	string	prometheus.io/port
17-23	object	resources
  18-20	object	requests
    19-19	string	cpu
    20-20	string	memory
  21-23	object	limits
    22-22	string	cpu
    23-23	string	memory
24-28	object	autoscaling
  25-25	bool	enabled
  26-26	number	minReplicas
  27-27	number	maxReplicas
  28-28	number	targetCPUUtilizationPercentage
29-33	object	persistence
  30-30	bool	enabled
  31-31	string	storageClass
  32-32	string	size
  33-33	string	mountPath
34-37	object	env
  35-35	string	logLevel
  36-36	string	cacheDir
  37-37	string	extraFlags
38-40	object	nodeSelector
  39-39	string	kubernetes.io/os
  40-40	string	node.example.com/pool
```

## Tiny files are dumped whole

For a very small file the structural outline can be *larger* than the file
itself (more line-range/kind/name rows than there are source lines). In that case
`--outline` skips the map and prints the raw file — same `path\tlanguage` header,
then the contents — so you never pay more tokens than just reading the file.

`deployment.yaml` is small enough to trigger this (its 24 lines outline to ~30
rows), so `grepple -O` returns the file as-is:

```bash
grepple -O sample-files/deployment.yaml
```

```text
sample-files/deployment.yaml	yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grepple-shard
  labels:
    app: grepple
spec:
  replicas: 3
  selector:
    matchLabels:
      app: grepple
  template:
    metadata:
      labels:
        app: grepple
    spec:
      containers:
        - name: shard
          image: grepple/shard:1.4.0
          ports:
            - containerPort: 8080
          env:
            - name: LOG_LEVEL
              value: info
```

> The comparison is per file and byte-based, so in a multi-file `--outline` run
> some files come back as compact maps and tiny ones come back whole. `--json`
> output is unaffected — it always returns the structured symbols.

## Markdown heading outline

Markdown outlines list the heading hierarchy (`h1`…`h6`):

```bash
grepple -O sample-files/guide.md
```

```text
sample-files/guide.md	markdown
1-20	h1	Search Guide
  5-12	h2	Configuration
    9-12	h3	TLS
  13-20	h2	Querying
    17-20	h3	Limits
```
