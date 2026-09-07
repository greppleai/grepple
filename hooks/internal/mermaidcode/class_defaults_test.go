package mermaidcode

import (
	"strings"
	"testing"
)

func TestClassMetadataDefaultsApplyDuringFinalization(t *testing.T) {
	diagram := `classDiagram
 class Record {
  +ID: string
 }
 <<struct>> Record
 class Store
 <<interface>> Store
 class Build
 <<function>> Build
 %% grepple:exact-default
 %% grepple:language-default go
 %% grepple:package-default example.com/app/model
`
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Record", "Store", "Build"} {
		class := parsed.Classes[name]
		if class.Package != "example.com/app/model" || class.Language != "go" {
			t.Fatalf("defaults not applied to %s: %+v", name, class)
		}
	}
	if !parsed.Classes["Record"].Exact || !parsed.Classes["Store"].Exact {
		t.Fatal("exact-default did not apply to struct and interface")
	}
	if parsed.Classes["Build"].Exact {
		t.Fatal("exact-default applied to a function")
	}
}

func TestClassMetadataDefaultsAreStrictAndOrderIndependent(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"malformed package", "%% grepple:package-default", "malformed package-default"},
		{"malformed language", "%% grepple:language-default rust", "malformed language-default"},
		{"malformed exact", "%% grepple:exact-default yes", "malformed exact-default"},
		{"duplicate package", "%% grepple:package-default one\n%% grepple:package-default two", "duplicate package-default"},
		{"duplicate language", "%% grepple:language-default go\n%% grepple:language-default go", "duplicate language-default"},
		{"duplicate exact", "%% grepple:exact-default\n%% grepple:exact-default", "duplicate exact-default"},
		{"global conflict", "%% grepple:package-default example.com/app\n%% grepple:language-default typescript", "conflicts with language-default"},
		{"package conflict default last", "class Item\n%% grepple:package Item example.com/other\n%% grepple:package-default example.com/app", "conflicts with package-default"},
		{"package conflict default first", "%% grepple:package-default example.com/app\nclass Item\n%% grepple:package Item example.com/other", "conflicts with package-default"},
		{"module conflict", "class Item\n%% grepple:module Item item.ts\n%% grepple:package-default example.com/app", "conflicts with package-default"},
		{"language conflict", "class Item\n<<struct>> Item\n%% grepple:language-default typescript", "conflicts with language-default"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseClassDiagram("classDiagram\n " + strings.ReplaceAll(test.body, "\n", "\n ") + "\n")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
