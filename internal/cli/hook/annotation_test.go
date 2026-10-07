package hook

import (
	"context"
	"go/format"
	"path/filepath"
	"strings"
	"testing"
)

func annotationRepository(t *testing.T) string {
	t.Helper()
	// Generic annotation engine fixture, independent of repository policies.
	yaml := `version: 1
id: test-attached-types
engine: gritql-v1
include: ["**/*.go"]
severity: error
unsuppressible: true
message: "named literals require an attached marker"
query: |
  language go
  or { struct_type(), interface_type() }
annotation:
  prefix: "//grepple: "
  values: [data, boundary, integration, use-case, storage, wire, runtime]
`
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/test-attached-types.yaml", string(yaml))
	return root
}

// grepple: data
// attachmentCase is one declaration policy fixture.
type attachmentCase struct {
	name, body string
	count      int
}

func TestAnnotationDeclarationAttachment(t *testing.T) {
	cases := []attachmentCase{
		{"missing struct", "type X struct{}", 1},
		{"missing interface", "type X interface{ Run() }", 1},
		{"valid", "//grepple: data\n// X documents the role.\ntype X struct{}", 0},
		{"following marker", "// X documents the role.\n//grepple: data\ntype X struct{}", 0},
		{"blank breaks attachment", "//grepple: data\n\ntype X struct{}", 1},
		{"unknown", "//grepple: mystery\ntype X struct{}", 1},
		{"case sensitive", "//grepple: Entity\ntype X struct{}", 1},
		{"multiple values", "//grepple: data port\ntype X struct{}", 1},
		{"duplicate", "//grepple: data\n//grepple: data\ntype X struct{}", 1},
		{"conflicting", "//grepple: data\n//grepple: boundary\ntype X struct{}", 1},
		{"invalid plus valid", "//grepple: mystery\n//grepple: data\ntype X struct{}", 1},
		{"block comment", "/* //grepple: data */\ntype X struct{}", 1},
		{"string marker", "const text = `//grepple: data`\ntype X struct{}", 1},
		{"trailing", "type X struct{} //grepple: data", 1},
		{"unrelated declaration", "//grepple: data\nconst A=1\ntype X struct{}", 1},
		{"group marker", "//grepple: data\ntype ( X struct{}; Y interface{} )", 2},
		{"group member markers", "type (\n//grepple: data\nX struct{}\n//grepple: boundary\nY interface{}\n)", 0},
		{"local missing", "func f(){type X struct{}}", 1},
		{"local marked", "func f(){\n//grepple: data\ntype X struct{}\n}", 0},
		{"literal alias", "//grepple: data\ntype X = struct{}", 0},
		{"literal interface alias", "//grepple: boundary\ntype X = interface{}", 0},
		{"parenthesized type", "//grepple: data\ntype X (struct{})", 0},
		{"parenthesized alias", "//grepple: boundary\ntype X = (interface{})", 0},
		{"wrong spacing", "//grepple:entity\ntype X struct{}", 1},
		{"generic", "//grepple: data\ntype X[T any] struct{ V T }", 0},
		{"anonymous struct", "//grepple: data\nvar x = struct{ V int }{}", 1},
		{"anonymous interface", "//grepple: boundary\nvar x interface{}", 1},
		{"nested anonymous", "//grepple: data\ntype X struct{ Nested struct{} }", 1},
		{"anonymous constraint", "func f[T interface{~string}](v T){}", 1},
		{"same line declarations", "//grepple: data\ntype X struct{}; type Y struct{}", 1},
		{"other directive", "//grepple:filelocal\n//grepple: data\ntype X struct{}", 0},
		{"physical positions", "//line other.go:999\n//grepple: data\ntype X struct{}", 0},
		{"named alias not resolved", "type X int\ntype Y = X", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := annotationRepository(t)
			writeHookTestFile(t, root, "p/types.go", "package p\n"+tc.body+"\n")
			report, status, err := runHookTest(t, "--all", "--id", "test-attached-types")
			if err != nil || len(report.Findings) != tc.count || (status != 0) != (tc.count != 0) {
				t.Fatalf("count=%d status=%d err=%v findings=%+v", tc.count, status, err, report.Findings)
			}
		})
	}
}

func TestAnnotationAllRolesAndCRLF(t *testing.T) {
	root := annotationRepository(t)
	for _, role := range []string{"data", "boundary", "integration", "use-case", "storage", "wire", "runtime"} {
		writeHookTestFile(t, root, filepath.Join(role, "types_test.go"), "package p\r\n//grepple: "+role+"\r\ntype X struct{}\r\n")
	}
	report, status, err := runHookTest(t, "--all", "--id", "test-attached-types")
	if err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("roles: %+v %d %v", report, status, err)
	}
}

func TestAnnotationCacheChangedFilesAndNoSuppression(t *testing.T) {
	root := annotationRepository(t)
	gitHookTest(t, root, "init", "-q")
	writeHookTestFile(t, root, "p/types.go", "package p\n//grepple: data\ntype X struct{}\n")
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "fixture")
	for _, body := range []string{"//grepple test-attached-types deliberate exception\ntype X struct{}", "//grepple: data\ntype X struct{}", "//grepple: mystery\ntype X struct{}"} {
		writeHookTestFile(t, root, "p/types.go", "package p\n"+body+"\n")
		want := 1
		if strings.HasPrefix(body, "//grepple: data") {
			want = 0
		}
		for range 2 {
			report, _, err := runHookTest(t, "--id", "test-attached-types")
			if err != nil || len(report.Findings) != want {
				t.Fatalf("cache/suppression: %+v %v", report, err)
			}
		}
	}
	// Full-snapshot integration sees unchanged violations, unlike changed mode.
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "unclassified")
	findings, err := CheckRepositoryFileRule(context.Background(), root, "test-attached-types")
	if err != nil || len(findings) != 1 {
		t.Fatalf("full snapshot: %+v %v", findings, err)
	}
}

func TestAnnotationRejectsInvalidSourceAndConfiguration(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		root := annotationRepository(t)
		writeHookTestFile(t, root, "p/types.go", "package p\n//grepple: data\ntype X struct{ broken\n")
		if _, _, err := runHookTest(t, "--all", "--id", "test-attached-types"); err == nil {
			t.Fatal("invalid source reported clean")
		}
	})
	for _, annotation := range []string{"prefix: x\n  values: [entity]", "prefix: '//grepple: '\n  values: []", "prefix: '//grepple: '\n  values: [entity, entity]", "prefix: '//grepple: '\n  values: [Entity]"} {
		t.Run(annotation, func(t *testing.T) {
			root := annotationRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/bad.yaml", "version: 1\nid: bad\nengine: gritql-v1\ninclude: ['**/*.go']\nseverity: error\nmessage: bad\nquery: |\n  language go\n  struct_type()\nannotation:\n  "+annotation+"\n")
			if _, _, err := runHookTest(t, "--all", "--id", "bad"); err == nil {
				t.Fatal("invalid annotation configuration accepted")
			}
		})
	}
}

func TestAnnotationRejectsUnsupportedEnginesAndLanguages(t *testing.T) {
	for _, engine := range []string{"gritql-relational-v1", "gritql-metric-v1"} {
		t.Run(engine, func(t *testing.T) {
			root := annotationRepository(t)
			config := "version: 1\nid: unsupported\nengine: " + engine + "\ninclude: ['**/*.go']\nseverity: error\nmessage: bad\nquery: |\n  language go\n  struct_type()\nannotation:\n  prefix: '//grepple: '\n  values: [entity]\n"
			writeHookTestFile(t, root, ".grepple/hooks/unsupported.yaml", config)
			if _, _, err := runHookTest(t, "--all", "--id", "unsupported"); err == nil || !strings.Contains(err.Error(), "annotation requires") {
				t.Fatalf("unsupported engine: %v", err)
			}
		})
	}
	t.Run("language", func(t *testing.T) {
		root := annotationRepository(t)
		config := "version: 1\nid: unsupported\nengine: gritql-v1\ninclude: ['**/*.ts']\nseverity: error\nmessage: bad\nquery: |\n  language typescript\n  interface_declaration()\nannotation:\n  prefix: '//grepple: '\n  values: [port]\n"
		writeHookTestFile(t, root, ".grepple/hooks/unsupported.yaml", config)
		if _, _, err := runHookTest(t, "--all", "--id", "unsupported"); err == nil || !strings.Contains(err.Error(), "only Go") {
			t.Fatalf("unsupported language: %v", err)
		}
	})
}

func TestAnnotationSurvivesGofmt(t *testing.T) {
	root := annotationRepository(t)
	input := []byte("package p\n//grepple: data\n// X is documented.\ntype X struct{}\n//grepple: boundary\ntype P interface{}\n")
	formatted, err := format.Source(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(formatted), "// grepple: data") {
		t.Fatalf("missing gofmt interaction: %s", formatted)
	}
	writeHookTestFile(t, root, "p/types.go", string(formatted))
	report, status, err := runHookTest(t, "--all", "--id", "test-attached-types")
	if err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("gofmt lost classification: %+v %v", report, err)
	}
	writeHookTestFile(t, root, "p/types.go", "package p\n//grepple: data\n// grepple: data\ntype X struct{}\n")
	report, _, err = runHookTest(t, "--all", "--id", "test-attached-types")
	if err != nil || len(report.Findings) != 1 {
		t.Fatalf("mixed spelling duplicate accepted: %+v %v", report, err)
	}
}
