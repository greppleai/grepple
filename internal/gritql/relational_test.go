package gritql

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func relationProgram(t *testing.T, query string) *Program {
	t.Helper()
	program, err := Compile([]byte(query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestCrossFileDuplicatesAcrossAllLanguages(t *testing.T) {
	sources := []struct{ language, source string }{
		{"c", "int value = 1;\n"}, {"cpp", "int value = 1;\n"},
		{"csharp", "class App { int value = 1; }\n"},
		{"dart", "void run() {}\n"},
		{"hcl", "locals { value = 1 }\n"},
		{"go", "package demo\nvar value = 1\n"},
		{"java", "class App { int value = 1; }\n"},
		{"javascript", "const value = 1;\n"},
		{"kotlin", "val value = 1\n"},
		{"php", "<?php function run() {}\n"},
		{"python", "value = 1\n"},
		{"rust", "fn run() { let value = 1; }\n"},
		{"shell", "value=1\n"},
		{"swift", "func run() {}\n"},
		{"tsx", "const value = 1;\n"},
		{"typescript", "const value = 1;\n"},
	}
	if len(sources) != len(SupportedLanguages()) {
		t.Fatalf("languages=%d sources=%d", len(SupportedLanguages()), len(sources))
	}
	for _, test := range sources {
		t.Run(test.language, func(t *testing.T) {
			// The root's first child is a declaration/statement in every adapter.
			// Matching its normalized syntax shows duplicates across files, not text.
			query := "language " + test.language + "\n" + map[string]string{
				"c": "translation_unit($statement)", "cpp": "translation_unit($statement)",
				"csharp": "compilation_unit($statement)", "dart": "source_file($statement)", "go": "source_file($statement)",
				"hcl":  "config_file($statement)",
				"java": "program($statement)", "javascript": "program($statement)",
				"kotlin": "source_file($statement)", "php": "program($statement)", "python": "module($statement)",
				"rust": "source_file($statement)", "shell": "program($statement)", "swift": "source_file($statement)",
				"tsx": "program($statement)", "typescript": "program($statement)",
			}[test.language]
			program := relationProgram(t, query)
			filesystem := fstest.MapFS{"pkg/a.src": {Data: []byte(test.source)}, "pkg/b.src": {Data: []byte(test.source)}}
			candidates := []ScanCandidate{{Path: "pkg/a.src", ReadPath: "pkg/a.src", Language: test.language}, {Path: "pkg/b.src", ReadPath: "pkg/b.src", Language: test.language}}
			spec := RelationSpec{Left: program, Right: program, LeftKey: RelationKey{Binding: "statement"}, RightKey: RelationKey{Binding: "statement"}, Scope: "directory"}
			hits, err := ScanFilesRelation(context.Background(), filesystem, candidates, spec, ScanOptions{})
			if err != nil || len(hits) != 2 || hits[0].Right.Path() != "pkg/a.src" || hits[1].Right.Path() != "pkg/b.src" {
				t.Fatalf("hits=%v err=%v", hits, err)
			}
		})
	}
}

func TestRelationGoStructMethodJoinAndAmbiguity(t *testing.T) {
	files := fstest.MapFS{
		"pkg/types.go":         {Data: []byte("package a\ntype (\n thing struct{}\n pair[A,B any] struct{}\n)\n")},
		"pkg/variants.go":      {Data: []byte("package a\ntype thing struct{}\n")},
		"pkg/method.go":        {Data: []byte("package a\nfunc (t *thing) Ambiguous() {}\nfunc (p *pair[A,B]) Valid() {}\n")},
		"pkg/external_test.go": {Data: []byte("package a_test\nfunc (p pair[A,B]) External() {}\n")},
		"pkg/same.go":          {Data: []byte("package a\ntype same struct{}\nfunc (s same) SameFile() {}\n")},
		"other/method.go":      {Data: []byte("package a\nfunc (p pair[A,B]) OtherDirectory() {}\n")},
	}
	var candidates []ScanCandidate
	for path := range files {
		candidates = append(candidates, ScanCandidate{Path: path, ReadPath: path})
	}
	left := relationProgram(t, "language go\ntype_spec(name=$name, type=struct_type())")
	right := relationProgram(t, "language go\nmethod_declaration(name=$method, receiver=parameter_list(parameter_declaration(type=$receiver)))")
	partition := relationProgram(t, "language go\npackage_clause($package)")
	spec := RelationSpec{Left: left, Right: right, Partition: partition, LeftKey: RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "receiver", DescendantKind: "type_identifier"}, PartitionKey: RelationKey{Binding: "package"}, Scope: "directory", UniqueLeft: true}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	if hit := hits[0]; hit.Left.Path() != "pkg/types.go" || hit.Right.Path() != "pkg/method.go" || hit.KeyText != "pair" || !strings.Contains(hit.Right.Text(), "Valid") {
		t.Fatalf("unexpected join: %+v", hit)
	}
}

func TestRelationFailsClosedOnMissingSourceAndTruncation(t *testing.T) {
	program := relationProgram(t, "language go\nsource_file($statement)")
	spec := RelationSpec{Left: program, Right: program, LeftKey: RelationKey{Binding: "statement"}, RightKey: RelationKey{Binding: "statement"}, Scope: "repository"}
	for name, candidates := range map[string][]ScanCandidate{
		"missing": {{Path: "bad.go", ReadPath: "bad.go"}},
		"syntax":  {{Path: "bad.go", ReadPath: "bad.go"}},
	} {
		files := fstest.MapFS{}
		if name == "syntax" {
			files["bad.go"] = &fstest.MapFile{Data: []byte("package demo\nfunc broken( {\n")}
		}
		if _, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{}); err == nil {
			t.Fatalf("%s: expected incomplete scan error", name)
		}
	}
}

func TestRelationRejectsTruncationAndCancellation(t *testing.T) {
	program := relationProgram(t, "language go\nsource_file($statement)")
	spec := RelationSpec{Left: program, Right: program, LeftKey: RelationKey{Binding: "statement"}, RightKey: RelationKey{Binding: "statement"}, Scope: "repository"}
	files := fstest.MapFS{"a.go": {Data: []byte("package demo\nvar a=1\n")}, "b.go": {Data: []byte("package demo\nvar b=2\n")}}
	candidates := []ScanCandidate{{Path: "a.go", ReadPath: "a.go"}, {Path: "b.go", ReadPath: "b.go"}}
	if _, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{MaxFiles: 1}); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("truncation error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScanFilesRelation(ctx, files, nil, spec, ScanOptions{}); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestRelationRejectsInvalidStaticSelectors(t *testing.T) {
	goProgram := relationProgram(t, "language go\nsource_file($statement)")
	tsProgram := relationProgram(t, "language typescript\nprogram($statement)")
	spec := RelationSpec{Left: goProgram, Right: tsProgram, LeftKey: RelationKey{Binding: "statement"}, RightKey: RelationKey{Binding: "statement"}, Scope: "repository"}
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "same language") {
		t.Fatalf("mixed-language validation=%v", err)
	}
	spec.Right = goProgram
	spec.RightKey.DescendantKind = "not_a_node_kind"
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "descendant kind") {
		t.Fatalf("unknown-kind validation=%v", err)
	}
}
