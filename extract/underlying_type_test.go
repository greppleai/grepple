package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func underlyingTestSource(t *testing.T) Source {
	t.Helper()
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	return Source{Path: filepath.Join(root, "model", "model.go"), Text: `package model

import grepple "example.com/shared"

type FileResult = grepple.FileResult
type ID string
type Records []ID
type Callback func(string) error
`}
}

func TestGoDeclarationsCaptureNormalizedUnderlyingTypes(t *testing.T) {
	source := underlyingTestSource(t)
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FileResult": "grepple.FileResult",
		"ID":         "string",
		"Records":    "[]ID",
		"Callback":   "func(string)error",
	}
	for name, expected := range want {
		declaration := analysis.Declarations[name]
		if declaration == nil || declaration.Underlying != expected {
			t.Errorf("%s underlying: got %+v, want %q", name, declaration, expected)
		}
	}
}

func TestUnderlyingMetadataValidatesAliasesAndNamedTypes(t *testing.T) {
	source := underlyingTestSource(t)
	diagram := `classDiagram
 class FileResult
 <<alias>> FileResult
 %% grepple:package FileResult example.com/app/model
 %% grepple:underlying FileResult grepple.FileResult
 class ID
 <<type>> ID
 %% grepple:package ID example.com/app/model
 %% pi:underlying ID string
 class Records
 <<type>> Records
 %% grepple:package Records example.com/app/model
 %% grepple:underlying Records []ID
 class Callback
 <<type>> Callback
 %% grepple:package Callback example.com/app/model
 %% grepple:underlying Callback func(string) error
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("validation: %v %+v", err, diagnostics)
	}
}

func TestUnderlyingMetadataReportsMismatch(t *testing.T) {
	source := underlyingTestSource(t)
	diagram := `classDiagram
 class Records
 <<type>> Records
 %% grepple:package Records example.com/app/model
 %% grepple:underlying Records []string
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 1 {
		t.Fatalf("validation: %v %+v", err, diagnostics)
	}
	if diagnostics[0].Line != 5 || !strings.Contains(diagnostics[0].Message, "Expected underlying type of 'Records'") || !strings.Contains(diagnostics[0].Message, "found '[]ID'") {
		t.Fatalf("unexpected diagnostic: %+v", diagnostics[0])
	}
}

func TestUnderlyingMetadataIsStrict(t *testing.T) {
	tests := []struct {
		name, directives, want string
	}{
		{"missing expression", "%% grepple:underlying Item", "malformed underlying directive"},
		{"invalid expression", "%% grepple:underlying Item []", "malformed underlying directive"},
		{"trailing input", "%% grepple:underlying Item string extra", "malformed underlying directive"},
		{"duplicate legacy", "%% grepple:underlying Item string\n%% pi:underlying Item int", "duplicate underlying directive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := "classDiagram\nclass Item\n<<type>> Item\n" + test.directives + "\n"
			_, err := ParseClassDiagram(diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestUnderlyingMetadataRequiresAliasOrNamedTypeNode(t *testing.T) {
	source := Source{Path: "model.go", Text: "package model\n\ntype Item struct{}\n"}
	diagram := "classDiagram\nclass Item\n<<struct>> Item\n%% grepple:underlying Item struct{}\n"
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "requires a Go <<type>> or <<alias>>") {
		t.Fatalf("validation: %v %+v", err, diagnostics)
	}
}

func TestUnderlyingMetadataGenerationRoundTrips(t *testing.T) {
	source := underlyingTestSource(t)
	for _, name := range []string{"FileResult", "ID", "Records", "Callback"} {
		generated, err := GenerateClassDiagram(name, source, []Source{source}, GenerateOptions{})
		if err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
		analysis, err := Analyze([]Source{source})
		if err != nil {
			t.Fatal(err)
		}
		expected := "%% grepple:underlying " + name + " " + analysis.Declarations[name].Underlying
		if !strings.Contains(generated, expected) {
			t.Fatalf("generated %s missing %q:\n%s", name, expected, generated)
		}
		diagnostics, err := CheckClassDiagram(generated, []Source{source})
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("round trip %s: %v %+v\n%s", name, err, diagnostics, generated)
		}
	}
}
