package search

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func withRepoRoot(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(old, ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func TestSearchAndStructuredSegments(t *testing.T) {
	withRepoRoot(t)
	p := Params{Query: "formatUser", Globs: []string{"testdata/typescript/*.ts"}, Regex: true, MaxSegments: 20}
	matches, err := Files(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%d", len(matches))
	}
	segments := matches[0].Segments
	wantSegments := []parser.Segment{
		{Kind: "summary", Start: 1, End: 1, Text: `import { logger } from "./logger";`},
		{Kind: "summary", Start: 3, End: 6, Text: "type User = { … }"},
		{Kind: "lines", Start: 8, End: 12},
		{Kind: "lines", Start: 14, End: 14},
		{Kind: "summary", Start: 15, End: 15, Text: "  constructor(private readonly users: User[]) {}"},
		{Kind: "summary", Start: 17, End: 19, Text: "  findUser(id: string): User | undefined { … }"},
		{Kind: "lines", Start: 21, End: 26},
	}
	if !reflect.DeepEqual(segments, wantSegments) {
		t.Fatalf("segments mismatch:\n got: %#v\nwant: %#v", segments, wantSegments)
	}
	var rendered strings.Builder
	lines := SplitLines(matches[0].Content)
	for _, s := range segments {
		if s.Kind == "summary" {
			rendered.WriteString(s.Text)
		} else {
			rendered.WriteString(strings.Join(lines[s.Start-1:s.End], "\n"))
		}
		rendered.WriteByte('\n')
	}
	got := rendered.String()
	for _, want := range []string{"export function formatUser", `logger.info("formatUser"`, "export class UserService", `console.log(formatUser(user))`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestCompactTSX(t *testing.T) {
	withRepoRoot(t)
	p := Params{Query: "data-testid='save-dashboard'", Globs: []string{"testdata/typescript/large-component.tsx"}, Regex: true, MaxSegments: 20}
	m, err := Files(p, nil)
	if err != nil || len(m) != 1 {
		t.Fatalf("%v matches=%d", err, len(m))
	}
	s := m[0].Segments
	var text strings.Builder
	lines := SplitLines(m[0].Content)
	for _, x := range s {
		if x.Kind == "summary" {
			text.WriteString(x.Text)
		} else {
			text.WriteString(strings.Join(lines[x.Start-1:x.End], "\n"))
		}
	}
	got := text.String()
	if !strings.Contains(got, "displayName") || !strings.Contains(got, "save-dashboard") {
		t.Fatalf("unexpected segments: %s", got)
	}
	if strings.Contains(got, "Last updated today") {
		t.Fatalf("footer should be collapsed: %s", got)
	}
}
