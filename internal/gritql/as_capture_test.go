package gritql

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestAsCapturesGoTypeIdentifierAcrossSourceDirectories(t *testing.T) {
	left := relationProgram(t, "language go\ntype_spec(name=$name) where { $name <: r\"^[A-Z]\" }")
	right := relationProgram(t, "language go\nand { type_identifier() as $name, not within type_spec(name=$name), } where { $name <: r\"^[A-Z]\" }")
	if !right.Features().Has(FeatureAs) || len(right.Variables()) != 1 {
		t.Fatalf("capture features=%v variables=%v", right.Features(), right.Variables())
	}
	files := fstest.MapFS{
		"internal/lib/types.go": {Data: []byte("package lib\ntype Used struct{}\ntype Unused struct{}\n")},
		"cmd/tool/main.go":      {Data: []byte("package main\nimport \"example.com/demo/internal/lib\"\nvar _ lib.Used\ntype Outside struct{}\n")},
	}
	candidates := []ScanCandidate{{Path: "internal/lib/types.go", ReadPath: "internal/lib/types.go"}, {Path: "cmd/tool/main.go", ReadPath: "cmd/tool/main.go"}}
	spec := RelationSpec{Left: left, Right: right, LeftKey: RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"}, Scope: "repository", Mode: "unmatched_left", LeftInclude: []string{"internal/**/*.go"}}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 1 || hits[0].Left.Path() != "internal/lib/types.go" || hits[0].KeyText != "Unused" {
		t.Fatalf("cross-directory type references: hits=%+v err=%v", hits, err)
	}
	files["cmd/tool/broken.go"] = &fstest.MapFile{Data: []byte("package main\nfunc broken( {\n")}
	candidates = append(candidates, ScanCandidate{Path: "cmd/tool/broken.go", ReadPath: "cmd/tool/broken.go"})
	if _, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{}); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("broken reference source excluded only from declarations must fail: %v", err)
	}
}

func TestAsCapturesGoPrivateTypeAndIgnoresExcludedTests(t *testing.T) {
	files := fstest.MapFS{
		"pkg/types.go":      {Data: []byte("package pkg\ntype hidden struct{}\ntype kept struct{}\nvar _ kept\n")},
		"pkg/types_test.go": {Data: []byte("package pkg\nvar _ hidden\n")},
	}
	candidates := []ScanCandidate{{Path: "pkg/types.go", ReadPath: "pkg/types.go"}, {Path: "pkg/types_test.go", ReadPath: "pkg/types_test.go"}}
	spec := RelationSpec{
		Left:      relationProgram(t, "language go\ntype_spec(name=$name) where { $name <: r\"^[a-z]\" }"),
		Right:     relationProgram(t, "language go\nand { type_identifier() as $name, not within type_spec(name=$name), } where { $name <: r\"^[a-z]\" }"),
		Partition: relationProgram(t, "language go\npackage_clause($package)"),
		LeftKey:   RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"}, PartitionKey: RelationKey{Binding: "package"},
		Scope: "directory", Mode: "unmatched_left",
	}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{ExcludeGlobs: []string{"**/*_test.go"}})
	if err != nil || len(hits) != 1 || hits[0].KeyText != "hidden" {
		t.Fatalf("private type candidates: hits=%+v err=%v", hits, err)
	}
}

func TestAsCaptureAndLeftScopeRejectInvalidConfiguration(t *testing.T) {
	for _, query := range []string{
		"language go\n`x` as $name",
		"language go\ntype_identifier() as $_",
	} {
		if _, err := Compile([]byte(query), CompileOptions{}); err == nil {
			t.Fatalf("unsupported capture accepted: %q", query)
		}
	}
	program := relationProgram(t, "language go\ntype_spec(name=$name)")
	for _, glob := range []string{"../outside/*.go", "[invalid", "/absolute/*.go"} {
		spec := RelationSpec{Left: program, Right: program, LeftKey: RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"}, Scope: "repository", LeftInclude: []string{glob}}
		if err := spec.Validate(); err == nil {
			t.Fatalf("invalid left_include %q accepted", glob)
		}
	}
}

func TestAsCapturesTypeScriptIdentifier(t *testing.T) {
	files := fstest.MapFS{
		"src/defs.ts": {Data: []byte("const Used = 1; const Unused = 2;\n")},
		"src/use.ts":  {Data: []byte("console.log(Used);\n")},
	}
	candidates := []ScanCandidate{{Path: "src/defs.ts", ReadPath: "src/defs.ts"}, {Path: "src/use.ts", ReadPath: "src/use.ts"}}
	spec := RelationSpec{
		Left:    relationProgram(t, "language typescript\nvariable_declarator(name=$name) where { $name <: r\"^[A-Z]\" }"),
		Right:   relationProgram(t, "language typescript\nand { identifier() as $name, not within variable_declarator(name=$name), } where { $name <: r\"^[A-Z]\" }"),
		LeftKey: RelationKey{Binding: "name"}, RightKey: RelationKey{Binding: "name"}, Scope: "repository", Mode: "unmatched_left",
	}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil || len(hits) != 1 || hits[0].KeyText != "Unused" {
		t.Fatalf("typescript root capture: hits=%+v err=%v", hits, err)
	}
}
