package mermaidcode

import (
	"strings"
	"testing"
)

func TestClassParserRejectsDuplicateSingletonMetadata(t *testing.T) {
	tests := []struct {
		name, directives, want string
	}{
		{"package", "%% grepple:package Item example.com/one\n%% pi:package Item example.com/two", "duplicate package"},
		{"module", "%% grepple:module Item one.ts\n%% pi:module Item two.ts", "duplicate module"},
		{"file", "%% grepple:file Item one.ts\n%% pi:file Item two.ts", "duplicate file"},
		{"filelocal", "%% grepple:filelocal Item\n%% pi:filelocal Item", "duplicate filelocal"},
		{"import", "%% grepple:import Item from one\n%% pi:import Item from two", "duplicate import"},
		{"default export", "%% grepple:default-export Item\n%% pi:default-export Item", "duplicate default-export"},
		{"package then module", "%% grepple:package Item example.com/app\n%% pi:module Item item.ts", "conflicts"},
		{"module then package", "%% grepple:module Item item.ts\n%% pi:package Item example.com/app", "conflicts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := "classDiagram\nclass Item\n" + test.directives + "\n"
			_, err := ParseClassDiagram(diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}
