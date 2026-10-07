package gritql

import "testing"

func TestCSSNativeNodesPreserveUTF8SourceRanges(t *testing.T) {
	program, err := Compile([]byte("language css\ndeclaration()"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "/* café */\n.button { content: \"héllo\"; }"
	result := EvaluateFile(t.Context(), program, FileInput{Path: "styles.css", Language: "css", Content: []byte(source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, nil)
	assertTargetFindings(t, result, []string{"content: \"héllo\";"})
	for _, finding := range result.Findings() {
		rangeValue := finding.Range()
		if string([]byte(source)[rangeValue.StartByte:rangeValue.EndByte]) != finding.Text() {
			t.Fatal("range does not reproduce source")
		}
	}
}

func TestCSSValueTemplatesDoNotMatchCommentsOrStrings(t *testing.T) {
	program, err := Compile([]byte("language css\n`color: $value;`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := ".button { color: red; content: \"color: blue;\"; } /* color: green; */"
	result := EvaluateFile(t.Context(), program, FileInput{Path: "styles.css", Language: "css", Content: []byte(source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, nil)
	assertTargetFindings(t, result, []string{"color: red;"})
}

func TestCSSRejectsForeignSyntaxAndInventedFields(t *testing.T) {
	for _, query := range []string{"language css\nfunction_declaration()", "language css\nrule_set(name=$name)"} {
		if _, err := Compile([]byte(query), CompileOptions{}); err == nil {
			t.Fatalf("invalid CSS contract accepted: %s", query)
		}
	}
}
