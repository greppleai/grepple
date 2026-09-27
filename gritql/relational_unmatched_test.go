package gritql

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestUnmatchedLeftMethodCandidatesUseCompleteDirectorySnapshot(t *testing.T) {
	files := fstest.MapFS{
		"pkg/defs.go": {Data: []byte(`package demo
 type target struct{}
 func (target) usedHere() {}
 func (target) usedElsewhere() {}
 func (target) usedAsValue() {}
 func (target) missing() {}
 func (target) Exported() {}
 func run(t target) { t.usedHere() }
 `)},
		"pkg/uses.go":   {Data: []byte("package demo\nfunc use(t target) { t.usedElsewhere(); _ = t.usedAsValue }\n")},
		"other/uses.go": {Data: []byte("package demo\nfunc use(t target) { t.missing() }\n")},
	}
	candidates := []ScanCandidate{
		{Path: "pkg/defs.go", ReadPath: "pkg/defs.go"},
		{Path: "pkg/uses.go", ReadPath: "pkg/uses.go"},
		{Path: "other/uses.go", ReadPath: "other/uses.go"},
	}
	spec := RelationSpec{
		Left:    relationProgram(t, "language go\nmethod_declaration(name=$name) where { $name <: r\"^[a-z]\" }"),
		Right:   relationProgram(t, "language go\nselector_expression(field=$name)"),
		LeftKey: RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"},
		Scope: "directory", Mode: "unmatched_left",
	}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 1 || hits[0].Left.Path() != "pkg/defs.go" || hits[0].KeyText != "missing" {
		t.Fatalf("unmatched candidates=%+v err=%v", hits, err)
	}
	// A reference in another file must suppress the finding; removing it must
	// expose the declaration, which requires scanning unchanged files as well.
	delete(files, "pkg/uses.go")
	hits, err = ScanFilesRelation(context.Background(), files, candidates[:1], spec, ScanOptions{})
	if err != nil || len(hits) != 3 {
		t.Fatalf("without cross-file uses: hits=%+v err=%v", hits, err)
	}
	for index, want := range []string{"usedElsewhere", "usedAsValue", "missing"} {
		if hits[index].KeyText != want {
			t.Fatalf("hit[%d]=%s want %s", index, hits[index].KeyText, want)
		}
	}
}

func TestUnmatchedLeftRejectsInvalidModeAndIncompleteScan(t *testing.T) {
	program := relationProgram(t, "language go\nsource_file($node)")
	spec := RelationSpec{Left: program, Right: program, LeftKey: RelationKey{Binding: "node"}, RightKey: RelationKey{Binding: "node"}, Scope: "repository", Mode: "unmatched_left"}
	files := fstest.MapFS{"a.go": {Data: []byte("package demo\nvar a = 1\n")}, "b.go": {Data: []byte("package demo\nvar b = 2\n")}}
	candidates := []ScanCandidate{{Path: "a.go", ReadPath: "a.go"}, {Path: "b.go", ReadPath: "b.go"}}
	if _, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{MaxFiles: 1}); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("truncated negative scan: %v", err)
	}
	files["b.go"] = &fstest.MapFile{Data: []byte("package demo\nfunc broken( {\n")}
	if _, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{}); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("broken source in negative scan: %v", err)
	}
	spec.Mode = "unknown"
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("unknown mode: %v", err)
	}
	spec.Mode, spec.UniqueLeft = "unmatched_left", true
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "unique_left") {
		t.Fatalf("unsupported ambiguity setting: %v", err)
	}
}

func TestUnmatchedLeftRespectsSourcePartitions(t *testing.T) {
	files := fstest.MapFS{
		"pkg/method.go":        {Data: []byte("package demo\ntype thing struct{}\nfunc (thing) missing() {}\n")},
		"pkg/external_test.go": {Data: []byte("package demo_test\nfunc run(t thing) { t.missing() }\n")},
	}
	candidates := []ScanCandidate{{Path: "pkg/method.go", ReadPath: "pkg/method.go"}, {Path: "pkg/external_test.go", ReadPath: "pkg/external_test.go"}}
	spec := RelationSpec{
		Left:      relationProgram(t, "language go\nmethod_declaration(name=$name)"),
		Right:     relationProgram(t, "language go\nselector_expression(field=$name)"),
		Partition: relationProgram(t, "language go\npackage_clause($package)"),
		LeftKey:   RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"}, PartitionKey: RelationKey{Binding: "package"},
		Scope: "directory", Mode: "unmatched_left",
	}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 1 || hits[0].KeyText != "missing" {
		t.Fatalf("partitioned missing method: hits=%+v err=%v", hits, err)
	}
	spec.Partition, spec.PartitionKey = nil, RelationKey{}
	hits, err = ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 0 {
		t.Fatalf("unpartitioned match: hits=%+v err=%v", hits, err)
	}
}
