package extract

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPackageGoTypeCardinality(t *testing.T) {
	tests := []struct {
		name        string
		typeValue   string
		cardinality string
		found       bool
	}{
		{"direct", "Target", "one", true},
		{"pointer", "*Target", "one", true},
		{"parentheses", "((*Target))", "one", true},
		{"slice", "[]Target", "many", true},
		{"array pointer element", "[3]*Target", "many", true},
		{"pointer to slice", "*[]Target", "many", true},
		{"nested array", "[2][3]Target", "many", true},
		{"map key", "map[Target]bool", "many", true},
		{"map value", "map[string]*Target", "many", true},
		{"map key and value", "map[Target]Target", "many", true},
		{"send channel", "chan<- *Target", "many", true},
		{"receive channel", "<-chan Target", "many", true},
		{"generic argument", "Box[Target]", "one", true},
		{"generic argument list", "Pair[string,*Target]", "one", true},
		{"generic repeated argument", "Box[[]Target]", "many", true},
		{"qualified generic local argument", "pkg.Box[Target]", "one", true},
		{"function parameter", "func(value Target) error", "one", true},
		{"function result", "func() *Target", "one", true},
		{"function repeated result", "func() []Target", "many", true},
		{"function in repeated container", "[]func(Target)", "many", true},
		{"anonymous struct", "struct{Value *Target}", "one", true},
		{"anonymous struct repeated field", "struct{Values []Target}", "many", true},
		{"anonymous interface method", "interface{Use(Target) error}", "one", true},
		{"anonymous interface repeated method", "interface{Values() chan Target}", "many", true},
		{"interface embedded type", "interface{Target}", "one", true},
		{"constraint approximation", "interface{~[]Target}", "many", true},
		{"constraint union", "interface{Target|[]byte}", "one", true},
		{"tuple direct", "tuple<Target,error>", "one", true},
		{"tuple repeated", "tuple<string,[]Target>", "many", true},
		{"mixed tuple is conservative", "tuple<Target,[]Target>", "many", true},
		{"mixed struct is conservative", "struct{One Target;Many []Target}", "many", true},
		{"variadic schema parameter", "...Target", "one", true},
		{"qualified target", "pkg.Target", "", false},
		{"selector qualifier", "Target.Member", "", false},
		{"field name", "struct{Target string}", "", false},
		{"parameter name", "func(Target string)", "", false},
		{"method name", "interface{Target() string}", "", false},
		{"array length", "[Target]string", "", false},
		{"identifier substring", "TargetList", "", false},
		{"invalid expression", "map[Target", "", false},
		{"empty", "", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cardinality, found := packageGoTypeCardinality(test.typeValue, "Target")
			if cardinality != test.cardinality || found != test.found {
				t.Fatalf("packageGoTypeCardinality(%q, Target) = %q, %v; want %q, %v", test.typeValue, cardinality, found, test.cardinality, test.found)
			}
		})
	}
}

func TestPackageManifestRelationsUseGoASTCardinality(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/cardinality\n")
	directory := filepath.Join(root, "model")
	writeScopeFile(t, filepath.Join(directory, "model.go"), `package model

type Target struct{}
type Box[T any] struct{}
type TargetList struct{}
type ManyAlias = *[]Target
type KeySet map[Target]bool

type Evidence struct {
	Direct *Target
	PointerToSlice *[]Target
	MapKey map[Target]bool
	Stream chan Target
	Generic Box[Target]
	Mixed struct { One Target; Many []Target }
	Target string
	Similar TargetList
	Callback func(Target) *Target
}

func Inspect(value struct { Item Target }, repeated []Target, generic Box[Target]) (Target, []Target) {
	return Target{}, nil
}
`)

	bundle, err := GeneratePackageBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	var manifest PackageIR
	if err := json.Unmarshal(bundle.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, relation := range manifest.Relations {
		if relation.To == "Target" {
			got[relation.From+"|"+relation.Via] = relation.Cardinality
		}
	}
	want := map[string]string{
		"ManyAlias|underlying type":     "many",
		"KeySet|underlying type":        "many",
		"Evidence|field Direct":         "one",
		"Evidence|field PointerToSlice": "many",
		"Evidence|field MapKey":         "many",
		"Evidence|field Stream":         "many",
		"Evidence|field Generic":        "one",
		"Evidence|field Mixed":          "many",
		"Evidence|field Callback":       "one",
		"Inspect|parameter 1":           "one",
		"Inspect|parameter 2":           "many",
		"Inspect|parameter 3":           "one",
		"Inspect|result 1":              "one",
		"Inspect|result 2":              "many",
	}
	if len(got) != len(want) {
		t.Fatalf("Target relation count = %d, want %d\ngot: %#v", len(got), len(want), got)
	}
	for evidence, cardinality := range want {
		if got[evidence] != cardinality {
			t.Errorf("relation %s cardinality = %q, want %q", evidence, got[evidence], cardinality)
		}
	}
	if _, exists := got["Evidence|field Target"]; exists {
		t.Error("field name produced a false Target relation")
	}
	if _, exists := got["Evidence|field Similar"]; exists {
		t.Error("TargetList produced a false Target relation")
	}
}
