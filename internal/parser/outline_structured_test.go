package parser

import "testing"

func TestOutlineJSON(t *testing.T) {
	src := `{
  "name": "grepple",
  "port": 8787,
  "debug": true,
  "scripts": {
    "build": "go build",
    "test": "go test"
  },
  "keywords": ["grep", "search"]
}
`
	o := OutlineFile("package.json", src)
	if o.Language != "json" {
		t.Fatalf("language = %q, want json", o.Language)
	}
	// Scalars are typed via the resolved tag.
	if s := mustFind(t, o.Symbols, "string", "name"); s.Start != 2 {
		t.Fatalf("name start = %d, want 2", s.Start)
	}
	mustFind(t, o.Symbols, "number", "port")
	mustFind(t, o.Symbols, "bool", "debug")
	// Nested object members hang off Children.
	scripts := mustFind(t, o.Symbols, "object", "scripts")
	mustFind(t, scripts.Children, "string", "build")
	mustFind(t, scripts.Children, "string", "test")
	// Arrays carry a length suffix and no per-element scalar noise.
	arr := mustFind(t, o.Symbols, "array", "keywords [2]")
	if len(arr.Children) != 0 {
		t.Fatalf("scalar array should have no children, got %d", len(arr.Children))
	}
}

func TestOutlineYAMLArrayOfObjects(t *testing.T) {
	src := `metadata:
  name: grepple
spec:
  ports:
    - name: http
      port: 8787
    - name: metrics
      port: 9090
`
	o := OutlineFile("svc.yaml", src)
	if o.Language != "yaml" {
		t.Fatalf("language = %q, want yaml", o.Language)
	}
	meta := mustFind(t, o.Symbols, "object", "metadata")
	if meta.Start != 1 || meta.End != 2 {
		t.Fatalf("metadata span = %d-%d, want 1-2", meta.Start, meta.End)
	}
	// Container elements of a sequence are shown as [i] and recursed into.
	ports := mustFind(t, o.Symbols, "array", "ports [2]")
	e0 := mustFind(t, ports.Children, "object", "[0]")
	mustFind(t, e0.Children, "string", "name")
	mustFind(t, e0.Children, "number", "port")
	mustFind(t, ports.Children, "object", "[1]")
}

func TestOutlineYAMLMultiDoc(t *testing.T) {
	src := `kind: Service
metadata:
  name: a
---
kind: ConfigMap
metadata:
  name: b
`
	o := OutlineFile("multi.yaml", src)
	d0 := mustFind(t, o.Symbols, "document", "[0]")
	d1 := mustFind(t, o.Symbols, "document", "[1]")
	// Each document's own members are nested under it.
	mustFind(t, d0.Children, "string", "kind")
	if _, ok := find(d0.Children, "object", "metadata"); !ok {
		t.Fatalf("doc 0 should contain metadata")
	}
	// Document [1] starts after the --- separator (line 5).
	if d1.Start != 5 {
		t.Fatalf("doc[1] start = %d, want 5", d1.Start)
	}
	meta1 := mustFind(t, d1.Children, "object", "metadata")
	if s := mustFind(t, meta1.Children, "string", "name"); s.Start != 7 {
		t.Fatalf("doc[1] metadata.name start = %d, want 7", s.Start)
	}
}

func TestOutlineStructuredDepthCap(t *testing.T) {
	src := `a:
  b:
    c: 1
`
	// Depth 1: only the top-level key, no descent.
	shallow := OutlineFileDepth("x.yaml", src, 1)
	a := mustFind(t, shallow.Symbols, "object", "a")
	if len(a.Children) != 0 {
		t.Fatalf("depth 1 should not expand children, got %d", len(a.Children))
	}
	// Depth 2: one more level.
	mid := OutlineFileDepth("x.yaml", src, 2)
	a2 := mustFind(t, mid.Symbols, "object", "a")
	b2 := mustFind(t, a2.Children, "object", "b")
	if len(b2.Children) != 0 {
		t.Fatalf("depth 2 should stop at b, got %d children", len(b2.Children))
	}
	// Unlimited: full descent to the scalar leaf.
	full := OutlineFile("x.yaml", src)
	mustFind(t, full.Symbols, "number", "c")
}

func TestOutlineStructuredEmptyOrInvalid(t *testing.T) {
	// A bare scalar document has no members to outline.
	if o := OutlineFile("v.yaml", "just a string\n"); len(o.Symbols) != 0 {
		t.Fatalf("scalar doc should yield no symbols, got %d", len(o.Symbols))
	}
	// Malformed input degrades to an empty (non-nil) outline, never a panic.
	o := OutlineFile("bad.yaml", "key: [unterminated\n")
	if o.Symbols == nil {
		t.Fatalf("symbols slice should be non-nil")
	}
}
