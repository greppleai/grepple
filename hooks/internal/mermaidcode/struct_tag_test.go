package mermaidcode

import (
	"strings"
	"testing"
)

func TestParseStructTagMetadata(t *testing.T) {
	diagram := "classDiagram\n" +
		" class Record\n" +
		" <<struct>> Record\n" +
		` %% grepple:struct-tag Record Name "json:\"name,omitempty\" validate:\"required\""` + "\n" +
		" %% pi:struct-tag Record Empty ``\n"
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		t.Fatalf("ParseClassDiagram: %v", err)
	}
	if got := parsed.Classes["Record"].StructTags["Name"].Value; got != `json:"name,omitempty" validate:"required"` {
		t.Fatalf("Name tag = %q", got)
	}
	if requirement, ok := parsed.Classes["Record"].StructTags["Empty"]; !ok || requirement.Value != "" {
		t.Fatalf("explicit empty requirement = %#v, %v", requirement, ok)
	}
}

func TestStructTagMetadataSyntaxIsStrict(t *testing.T) {
	tests := []struct {
		name, directive, want string
	}{
		{"missing tag", "%% grepple:struct-tag Record Name", "malformed struct-tag directive"},
		{"not quoted", "%% grepple:struct-tag Record Name json:name", "malformed struct-tag directive"},
		{"rune literal", "%% grepple:struct-tag Record Name 'x'", "malformed struct-tag directive"},
		{"trailing input", `%% grepple:struct-tag Record Name "json:name" extra`, "malformed struct-tag directive"},
		{"duplicate", "%% grepple:struct-tag Record Name \"one\"\n %% pi:struct-tag Record Name \"two\"", "duplicate struct-tag directive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagram := "classDiagram\n class Record\n <<struct>> Record\n " + test.directive + "\n"
			_, err := ParseClassDiagram(diagram)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestAnalyzeGoStructTagsDistinguishesAbsentAndEmpty(t *testing.T) {
	source := Source{"record.go", "package model\ntype Record struct {\n Absent string\n Empty string \"\"\n A, B int `json:\"count\"`\n}\n"}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	declaration := analysis.Declarations["Record"]
	if tag := declaration.StructTags["Absent"]; tag.Present {
		t.Fatalf("absent tag captured as present: %#v", tag)
	}
	if tag := declaration.StructTags["Empty"]; !tag.Present || tag.Value != "" {
		t.Fatalf("empty tag not preserved: %#v", tag)
	}
	for _, field := range []string{"A", "B"} {
		if tag := declaration.StructTags[field]; !tag.Present || tag.Value != `json:"count"` {
			t.Fatalf("%s tag = %#v", field, tag)
		}
	}
}

func TestValidateStructTagRequirements(t *testing.T) {
	source := Source{"record.go", "package model\ntype Record struct {\n Name string `json:\"name\"`\n Empty string \"\"\n Absent string\n}\n"}
	base := "classDiagram\n class Record\n <<struct>> Record\n"
	matching := base + ` %% grepple:struct-tag Record Name "json:\"name\""` + "\n %% grepple:struct-tag Record Empty \"\"\n"
	if diagnostics, err := CheckClassDiagram(matching, []Source{source}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("matching requirements: %v %+v", err, diagnostics)
	}

	tests := []struct {
		name, directive, want string
	}{
		{"wrong value", `%% grepple:struct-tag Record Name "json:\"other\""`, `found "json:\"name\""`},
		{"absent", `%% grepple:struct-tag Record Absent ""`, "found no struct tag"},
		{"unknown field", `%% grepple:struct-tag Record Missing ""`, "is not a named field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics, err := CheckClassDiagram(base+" "+test.directive+"\n", []Source{source})
			if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, test.want) {
				t.Fatalf("got %v %+v, want %q", err, diagnostics, test.want)
			}
		})
	}
}

func TestStructTagsAreOptInAndLegacyMetadataValidates(t *testing.T) {
	source := Source{"record.go", "package model\ntype Record struct { Name string `json:\"name\"` }\n"}
	base := "classDiagram\n class Record {\n  +Name: string\n }\n <<struct>> Record\n"
	if diagnostics, err := CheckClassDiagram(base, []Source{source}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("diagram without tag requirements: %v %+v", err, diagnostics)
	}
	legacy := base + ` %% pi:struct-tag Record Name "json:\"name\""` + "\n"
	if diagnostics, err := CheckClassDiagram(legacy, []Source{source}); err != nil || len(diagnostics) != 0 {
		t.Fatalf("legacy requirement: %v %+v", err, diagnostics)
	}
}

func TestGenerateGoStructTags(t *testing.T) {
	source := Source{"record.go", "package model\ntype Record struct {\n Name string `json:\"name,omitempty\" validate:\"required\"`\n Empty string \"\"\n Untagged int\n}\n"}
	generated, err := GenerateClassDiagram("Record", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateClassDiagram: %v", err)
	}
	for _, expected := range []string{
		`%% grepple:struct-tag Record Empty ""`,
		`%% grepple:struct-tag Record Name "json:\"name,omitempty\" validate:\"required\""`,
	} {
		if !strings.Contains(generated, expected) {
			t.Errorf("missing %q:\n%s", expected, generated)
		}
	}
	if strings.Contains(generated, "struct-tag Record Untagged") || strings.Contains(generated, "%% pi:struct-tag") {
		t.Fatalf("generated unwanted tag metadata:\n%s", generated)
	}
}
