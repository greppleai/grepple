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
			`[summary:3-3:"import \"fmt\""][summary:5-8:"type User struct { … }"][lines:10-12:""][summary:14-14:"type Reporter struct{}"][lines:16-18:""]`,
		},
		{
			"testdata/typescript/large-component.tsx", "data-testid",
			`[summary:1-5:"type DashboardProps = { … }"][lines:7-7:""][summary:8-8:"  const displayName = userName.trim() || 'friend';"][summary:9-9:"  const subtitle = ` + "`" + `Welcome ${displayName}` + "`" + `;"][lines:18-20:""][lines:27-27:""]`,
		},
		{
			"testdata/java/Sample.java", "println",
			`[summary:3-3:"import java.util.List;"][lines:5-5:""][summary:6-6:"    record User(String id, String name) {}"][summary:8-10:"    public static String formatUser(User user) { … }"][lines:12-17:""]`,
		},
		{
			"testdata/kotlin/Sample.kt", "println",
			`[summary:3-3:"data class User(val id: String, val name: String)"][lines:5-5:""][summary:6-8:"    fun findUser(id: String): User? { … }"][lines:10-14:""]`,
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
