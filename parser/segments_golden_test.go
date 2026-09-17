package parser

import (
	"fmt"
	"os"
	"reflect"
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
		for _, segment := range BuildSegments(content, LanguageFor(c.path), hits, 20) {
			got += fmt.Sprintf("[%s:%d-%d:%q]", segment.Kind, segment.Start, segment.End, segment.Text)
		}
		if got != c.want {
			t.Errorf("%s segments mismatch:\n got: %s\nwant: %s", c.path, got, c.want)
		}
	}
}

func TestASTSegmentLimitKeepsDirectMatchingCallable(t *testing.T) {
	withRepoRoot(t)
	contentBytes, err := os.ReadFile("testdata/java/Sample.java")
	if err != nil {
		t.Fatal(err)
	}
	segments := BuildSegments(string(contentBytes), "java", map[int]bool{14: true}, 1)
	want := []Segment{{Kind: "lines", Start: 12, End: 17}}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("segments=%#v want=%#v", segments, want)
	}
}
