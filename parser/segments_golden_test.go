package parser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestASTSegmentsGolden pins exact structural output for representative files.
func TestASTSegmentsGolden(t *testing.T) {
	withRepoRoot(t)
	cases := []struct {
		path, query, want string
	}{
		{
			"testdata/go/sample.go", "FormatUser",
			`[lines:10-12:""][lines:16-18:""]`,
		},
		{
			"testdata/typescript/large-component.tsx", "data-testid",
			`[lines:7-27:""]`,
		},
		{
			"testdata/java/Sample.java", "println",
			`[lines:5-5:""][lines:12-17:""]`,
		},
		{
			"testdata/kotlin/Sample.kt", "println",
			`[lines:5-5:""][lines:10-14:""]`,
		},
	}
	for _, c := range cases {
		contentBytes, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(contentBytes)
		hits := map[int]bool{}
		for i, line := range strings.Split(content, "\n") {
			if strings.Contains(line, c.query) {
				hits[i+1] = true
			}
		}
		var got string
		for _, segment := range BuildSegments(content, LanguageFor(c.path), hits) {
			got += fmt.Sprintf("[%s:%d-%d:%q]", segment.Kind, segment.Start, segment.End, segment.Text)
		}
		if got != c.want {
			t.Errorf("%s segments mismatch:\n got: %s\nwant: %s", c.path, got, c.want)
		}
	}
}

func TestBuildSegmentsKeepsEveryMatchingScope(t *testing.T) {
	var content strings.Builder
	content.WriteString("package p\n\n")
	hits := make(map[int]bool)
	for index := 0; index < 25; index++ {
		fmt.Fprintf(&content, "func f%d() {\n\tmarker%d := true\n}\n\n", index, index)
		hits[4+index*4] = true
	}
	segments := BuildSegments(content.String(), "go", hits)
	if len(segments) != 25 {
		t.Fatalf("segments=%d want=25: %#v", len(segments), segments)
	}
	for index, segment := range segments {
		wantStart := 3 + index*4
		if segment.Kind != "lines" || segment.Start != wantStart || segment.End != wantStart+2 {
			t.Fatalf("segment %d=%+v want lines %d-%d", index, segment, wantStart, wantStart+2)
		}
	}
}
