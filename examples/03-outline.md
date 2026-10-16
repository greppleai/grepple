# Outlines (`--outline` / `-O`)

`--outline` prints a file's declaration hierarchy. Parser-verified local rows have a reusable `NAME @KIND:START-END` selector; other rows keep `START-END<TAB>KIND<TAB>NAME`. Nested members are indented. JSON outlines remain structured and do not include these human selectors.

## Go symbols

```bash
grepple -O sample-files/server.go
```

```text
sample-files/server.go	go
Config @struct:9-12
Server @struct:15-18
NewServer @func:21-23
(*Server).Handle @method:26-28
(*Server).ListenAndServe @method:31-38
```

Go methods appear at top level as `(*Type).Method`, in source order.

## TSX symbols

```bash
grepple -O sample-files/Button.tsx
```

```text
sample-files/Button.tsx	tsx
3-6	type	ButtonProps
Button @function:8-21
```

## Java symbols

Classes nest their members (records, fields, constructors, methods) with line
ranges:

```bash
grepple -O sample-files/Order.java
```

```text
sample-files/Order.java	java
Order @class:5-25
  6-6	record	LineItem
  8-8	field	id
  9-9	field	items
  Order @method:11-14
  total @method:16-20
  describe @method:22-24
```

## Kotlin symbols

Top-level `data class` and `class` declarations, with member functions nested
under their class:

```bash
grepple -O sample-files/Inventory.kt
```

```text
sample-files/Inventory.kt	kotlin
Product @class:3-3
Inventory @class:5-19
  find @method:6-7
  restock @method:9-15
  lowStock @method:17-18
```

## JSON key/type tree

For JSON and YAML the outline is a key skeleton with **values omitted** — only keys and types. Arrays show their length as `[N]`; verified objects, arrays, mappings, and sequences can also have selectors.

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
features [3] @array:12-12
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
features [3] @array:12-12
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
image @mapping:3-9
  4-4	string	repository
  5-5	string	tag
  6-6	string	pullPolicy
  pullSecrets [2] @sequence:7-9
    [0] @mapping:8-8
      name @mapping:8-8
    [1] @mapping:9-9
      name @mapping:9-9
service @mapping:10-16
  11-11	string	type
  12-12	number	port
  annotations @mapping:13-16
    14-14	string	prometheus.io/scrape
    15-15	string	prometheus.io/path
    16-16	string	prometheus.io/port
resources @mapping:17-23
  requests @mapping:18-20
    19-19	string	cpu
    20-20	string	memory
  limits @mapping:21-23
    22-22	string	cpu
    23-23	string	memory
autoscaling @mapping:24-28
  25-25	bool	enabled
  26-26	number	minReplicas
  27-27	number	maxReplicas
  28-28	number	targetCPUUtilizationPercentage
persistence @mapping:29-33
  30-30	bool	enabled
  31-31	string	storageClass
  32-32	string	size
  33-33	string	mountPath
env @mapping:34-37
  35-35	string	logLevel
  36-36	string	cacheDir
  37-37	string	extraFlags
nodeSelector @mapping:38-40
  39-39	string	kubernetes.io/os
  40-40	string	node.example.com/pool
```

## Deployment YAML outline

The 46-line deployment fixture has nested metadata and security settings. Its outline includes verified mapping and sequence selectors along with ordinary scalar rows:

```bash
grepple -O sample-files/deployment.yaml
```

```text
sample-files/deployment.yaml	yaml
1-1	string	apiVersion
2-2	string	kind
metadata @mapping:3-6
  4-4	string	name
  labels @mapping:5-6
    6-6	string	app
spec @mapping:7-46
  8-8	number	replicas
  selector @mapping:9-11
    matchLabels @mapping:10-11
      11-11	string	app
  template @mapping:12-46
    metadata @mapping:13-15
      labels @mapping:14-15
        15-15	string	app
    spec @mapping:16-46
      securityContext @mapping:17-25
        18-18	bool	runAsNonRoot
        19-19	number	runAsUser
        20-20	number	runAsGroup
        21-21	number	fsGroup
        seccompProfile @mapping:22-23
          23-23	string	type
        appArmorProfile @mapping:24-25
          25-25	string	type
      containers [1] @sequence:26-46
        [0] @mapping:27-46
          27-27	string	name
          28-28	string	image
          ports [1] @sequence:29-30
            [0] @mapping:30-30
              containerPort @mapping:30-30
          env [1] @sequence:31-33
            [0] @mapping:32-33
              32-32	string	name
              33-33	string	value
          securityContext @mapping:34-46
            35-35	bool	runAsNonRoot
            36-36	number	runAsUser
            37-37	bool	allowPrivilegeEscalation
            38-38	bool	readOnlyRootFilesystem
            39-39	bool	privileged
            capabilities @mapping:40-42
              drop [1] @sequence:41-42
            seccompProfile @mapping:43-44
              44-44	string	type
            appArmorProfile @mapping:45-46
              46-46	string	type
```

> Human `--outline` retains selectable rows instead of falling back to raw file content, even for a small file. `--json` always returns structured symbols without human selector strings.

## Markdown heading outline

Markdown outlines list the heading hierarchy (`h1`…`h6`):

```bash
grepple -O sample-files/guide.md
```

```text
sample-files/guide.md	markdown
1-20	h1	Search Guide
  Configuration @section:5-12
    TLS @section:9-12
  13-20	h2	Querying
    17-20	h3	Limits
```
