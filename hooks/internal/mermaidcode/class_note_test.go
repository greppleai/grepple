package mermaidcode

import (
	"strings"
	"testing"
)

func TestParseClassDiagramNotes(t *testing.T) {
	diagram, err := ParseClassDiagram("classDiagram\n class Handler\n note \"summary &amp; counts\"\n note for Handler \"a \\\"quoted\\\" route\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(diagram.Notes) != 2 || diagram.Notes[0].Target != "" || diagram.Notes[0].Text != "summary &amp; counts" || diagram.Notes[1].Target != "Handler" || diagram.Notes[1].Text != `a "quoted" route` {
		t.Fatalf("notes = %#v", diagram.Notes)
	}
}

func TestParseClassDiagramNoteErrors(t *testing.T) {
	tests := []struct {
		name, note, want string
	}{
		{"unquoted", `note summary`, "malformed note"},
		{"single quoted", `note 'summary'`, "malformed note"},
		{"trailing tokens", `note "summary" extra`, "malformed note"},
		{"unterminated", `note "summary`, "malformed note"},
		{"missing target", `note for "summary"`, "malformed note"},
		{"undeclared target", `note for Missing "summary"`, "not declared"},
		{"target after note", "note for Later \"summary\"\n class Later", "not declared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseClassDiagram("classDiagram\n class Handler\n " + test.note + "\n")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestMermaidNoteTextIsSingleLineAndInert(t *testing.T) {
	got := mermaidNoteText("quoted \" & line\n\tend")
	if got != "quoted &quot; &amp; line&#10;&#9;end" || strings.ContainsAny(got, "\r\n\"") {
		t.Fatalf("escaped note = %q", got)
	}
}
