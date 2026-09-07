package mermaidcode

import (
	"strings"
	"testing"
)

func TestClassParserAllowsOrdinaryMermaidComments(t *testing.T) {
	diagram := `%% architecture overview
classDiagram
 %% package ownership: platform
 class User {
  %% properties exposed by the API
  +string Name
 }
 %% pi metadata is documented elsewhere
`
	if _, err := ParseClassDiagram(diagram); err != nil {
		t.Fatalf("ordinary comments must be allowed: %v", err)
	}
}

func TestClassParserRejectsUnknownNamespacedMetadata(t *testing.T) {
	tests := []struct {
		name, directive string
	}{
		{"grepple unknown", "%% grepple:endpoint User /users"},
		{"legacy pi unknown", "%% pi:endpoint User /users"},
		{"flow-only directive", "%% grepple:symbol User service.User"},
		{"before header", "%% pi:unknown value\nclassDiagram"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := "classDiagram\n class User\n " + test.directive + "\n"
			if test.name == "before header" {
				diagram = test.directive
			}
			_, err := ParseClassDiagram(diagram)
			if err == nil || !strings.Contains(err.Error(), "unknown metadata directive") {
				t.Fatalf("got %v, want unknown metadata directive error", err)
			}
		})
	}
}

func TestClassParserRejectsMalformedNamespacedMetadata(t *testing.T) {
	tests := []struct {
		directive, want string
	}{
		{"%% grepple:", "malformed metadata directive"},
		{"%% grepple:package User", "malformed package directive"},
		{"%% pi:module User", "malformed module directive"},
		{"%% grepple:import User from", "malformed import directive"},
		{"%% pi:filelocal User extra", "malformed filelocal directive"},
		{"%% grepple:generated entry User depth many max-nodes 20", "malformed generated directive"},
	}
	for _, test := range tests {
		t.Run(test.directive, func(t *testing.T) {
			diagram := "classDiagram\n class User\n " + test.directive + "\n"
			_, err := ParseClassDiagram(diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestClassParserAcceptsGeneratedMetadata(t *testing.T) {
	diagram := "classDiagram\n %% grepple:generated entry User depth 2 max-nodes 20\n class User\n"
	if _, err := ParseClassDiagram(diagram); err != nil {
		t.Fatalf("generated metadata: %v", err)
	}
}
