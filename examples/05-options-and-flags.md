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

4   2026-08-31T09:16:03Z ERROR upstream timeout path=/index status=504

// … 1 line collapsed …

6   2026-08-31T09:16:04Z INFO  request ok path=/index status=200
```

## Fixed strings (`-F`)

Treat the pattern as a literal substring — handy when it contains regex metacharacters
like `.`:

```bash
grepple -F "http.HandlerFunc" sample-files/server.go --limit 1
```

```text
sample-files/server.go


// … 8 lines collapsed …

 9   type Config struct { … }

// … 2 lines collapsed …

15   type Server struct {
16   	cfg    Config
17   	routes map[string]http.HandlerFunc
18   }

// … 2 lines collapsed …

21   func NewServer(cfg Config) *Server {
22   	return &Server{cfg: cfg, routes: map[string]http.HandlerFunc{}}
23   }

// … 2 lines collapsed …

26   func (s *Server) Handle(path string, fn http.HandlerFunc) {
27   	s.routes[path] = fn
28   }

// … 2 lines collapsed …

31   func (s *Server) ListenAndServe() error { … }
```

## Case-insensitive (`-i`)

`SERVER` matches `server`:

```bash
grepple -i "SERVER" sample-files/server.log
```

```text
sample-files/server.log

1   2026-08-31T09:15:01Z INFO  server starting addr=0.0.0.0:8080
```
