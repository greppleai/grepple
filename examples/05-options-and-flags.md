# Matching options & flags

How the pattern is interpreted. `--regex` is the default; `-F`/`--fixed-strings` and
`--regex` are mutually exclusive.

## Regex (default)

The pattern is a regular expression unless told otherwise:

```bash
grepple "status=\d+" sample-files/server.log
```

```text
sample-files/server.log


// … 3 lines collapsed …

epv│4│2026-08-31T09:16:03Z ERROR upstream timeout path=/index status=504

// … 1 line collapsed …

GXn│6│2026-08-31T09:16:04Z INFO  request ok path=/index status=200
```

## Fixed strings (`-F`)

Treat the pattern as a literal substring — handy when it contains regex metacharacters like `.`. `--limit 1` limits matching **files**, not the number of matching lines or declarations within this one file:

```bash
grepple -F "http.HandlerFunc" sample-files/server.go --limit 1
```

```text
sample-files/server.go


// … 13 lines collapsed …

zpE│14│// Server wraps the standard library HTTP server.
FQS│15│type Server struct {
Zhj│16│	cfg    Config
Xka│17│	routes map[string]http.HandlerFunc
hI6│18│}

XHd│20│// NewServer builds a Server from the given config.
ah3│21│func NewServer(cfg Config) *Server {
m5S│22│	return &Server{cfg: cfg, routes: map[string]http.HandlerFunc{}}
DVO│23│}

dcR│25│// Handle registers a handler for a path.
t8r│26│func (s *Server) Handle(path string, fn http.HandlerFunc) {
PCM│27│	s.routes[path] = fn
H9-│28│}

Next points (code navigation):
  → Config  sample-files/server.go:9-12  parameter-type:21
  → Server  sample-files/server.go:15-18  result-type:20
  → Server  sample-files/server.go:15-18  receiver-type:26

Related type definitions:

sample-files/server.go:9-12  Config
Rj1│9│type Config struct {
F3r│10│	Addr    string
swh│11│	Timeout int
AUS│12│}

sample-files/server.go:15-18  Server
FQS│15│type Server struct {
Zhj│16│	cfg    Config
Xka│17│	routes map[string]http.HandlerFunc
hI6│18│}
```

## Case-insensitive (`-i`)

`SERVER` matches `server`:

```bash
grepple -i "SERVER" sample-files/server.log
```

```text
sample-files/server.log

eAL│1│2026-08-31T09:15:01Z INFO  server starting addr=0.0.0.0:8080
```
