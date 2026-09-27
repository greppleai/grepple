package gritql

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRelationGoReturnTypesAnyInterface(t *testing.T) {
	files := fstest.MapFS{
		"pkg/functions.go": {Data: []byte(`package service
func Value() int { return 0 }
func None() {}
func Struct() Concrete { return Concrete{} }
func Pointer() *Concrete { return nil }
func Slice() []Remote { return nil }
func Qualified() imported.Remote { return nil }
func First() (Remote, Concrete) { return nil, Concrete{} }
func Last() (Concrete, Remote) { return Concrete{}, nil }
func Named() (value Concrete, svc Remote) { return Concrete{}, nil }
func Generic() GenericInterface[int] { return nil }
func Inline() interface{ Run() } { return nil }
func Builtin() (Concrete, error) { return Concrete{}, nil }
func Any() any { return nil }
func SameNameWrongPackage() Other { return nil }
func ManyConcrete() (Concrete, int) { return Concrete{}, 0 }
func (Concrete) Method() Concrete { return Concrete{} }
func init() {}
func main() {}
`)},
		"pkg/types.go":       {Data: []byte("package service\ntype Remote interface { Run() }\ntype GenericInterface[T any] interface { Run() }\ntype Concrete struct{}\n")},
		"pkg/other.go":       {Data: []byte("package different\ntype Other interface { Run() }\n")},
		"elsewhere/types.go": {Data: []byte("package service\ntype Other interface { Run() }\n")},
	}
	var candidates []ScanCandidate
	for name := range files {
		candidates = append(candidates, ScanCandidate{Path: name, ReadPath: name})
	}
	left := relationProgram(t, "language go\nand { function_declaration(name=$name), maybe function_declaration(result=$result), not `func init() { $body }`, not `func main() { $body }`, }")
	right := relationProgram(t, "language go\ntype_spec(name=$type, type=interface_type())")
	partition := relationProgram(t, "language go\npackage_clause($package)")
	spec := RelationSpec{Left: left, Right: right, Partition: partition, LeftKey: RelationKey{Binding: "result", Projection: "go-return-types"}, RightKey: RelationKey{Binding: "type"}, PartitionKey: RelationKey{Binding: "package"}, Scope: "directory", Mode: "unmatched_left_any"}
	hits, err := ScanFilesRelation(context.Background(), files, candidates, spec, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"func Value()", "func None()", "func Struct()", "func Pointer()", "func Slice()", "func Qualified()", "func SameNameWrongPackage()", "func ManyConcrete()"}
	if len(hits) != len(want) {
		t.Fatalf("hits=%d want=%d: %+v", len(hits), len(want), hits)
	}
	for i, hit := range hits {
		if hit.Left.Path() != "pkg/functions.go" || !strings.HasPrefix(hit.Left.Text(), want[i]) {
			t.Errorf("hit %d: %s %q, want %q", i, hit.Left.Path(), hit.Left.Text(), want[i])
		}
	}
}

func TestRelationGoReturnTypesRejectsUnscopedAndInvalidModes(t *testing.T) {
	left := relationProgram(t, "language go\nand { function_declaration(name=$name), maybe function_declaration(result=$result), }")
	right := relationProgram(t, "language go\ntype_spec(name=$type, type=interface_type())")
	partition := relationProgram(t, "language go\npackage_clause($package)")
	spec := RelationSpec{Left: left, Right: right, Partition: partition, LeftKey: RelationKey{Binding: "result", Projection: "go-return-types"}, RightKey: RelationKey{Binding: "type"}, PartitionKey: RelationKey{Binding: "package"}, Scope: "directory", Mode: "unmatched_left_any"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*RelationSpec)
	}{
		{"no projection", func(s *RelationSpec) { s.LeftKey.Projection = "" }},
		{"wrong mode", func(s *RelationSpec) { s.Mode = "unmatched_left" }},
		{"right projection", func(s *RelationSpec) { s.RightKey.Projection = "go-return-types" }},
		{"unique", func(s *RelationSpec) { s.UniqueLeft = true }},
		{"no partition", func(s *RelationSpec) { s.Partition, s.PartitionKey = nil, RelationKey{} }},
		{"repository scope", func(s *RelationSpec) { s.Scope = "repository" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := spec
			tc.edit(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected relation validation failure")
			}
		})
	}
}
