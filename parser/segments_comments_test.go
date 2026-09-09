package parser

import "testing"

func TestBuildSegmentsIncludesLeadingComments(t *testing.T) {
	tests := []struct {
		name     string
		language string
		content  string
		hitLine  int
		wantEnd  int
	}{
		{
			name:     "jsdoc",
			language: "javascript",
			content:  "/**\n * Returns a greeting.\n */\nfunction greet() {\n  return 'hello';\n}\n",
			hitLine:  5,
			wantEnd:  6,
		},
		{
			name:     "tsdoc",
			language: "typescript",
			content:  "/** Returns a greeting. */\nexport function greet(): string {\n  return 'hello';\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "go doc comment",
			language: "go",
			content:  "// Greet returns a greeting.\nfunc Greet() string {\n\treturn \"hello\"\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "java javadoc",
			language: "java",
			content:  "/** Greets users. */\nclass Greeter {\n  String message = \"hello\";\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "kotlin kdoc",
			language: "kotlin",
			content:  "/** Returns a greeting. */\nfun greet(): String {\n  return \"hello\"\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "csharp xml documentation",
			language: "csharp",
			content:  "/// <summary>Greets users.</summary>\nclass Greeter {\n  string message = \"hello\";\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "c doxygen",
			language: "c",
			content:  "/** Returns a greeting. */\nconst char *greet(void) {\n  return \"hello\";\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "cpp doxygen",
			language: "cpp",
			content:  "/// Returns a greeting.\nstd::string greet() {\n  return \"hello\";\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
		{
			name:     "python comment before decorator",
			language: "python",
			content:  "# Greet returns a greeting.\n@public\ndef greet():\n    return 'hello'\n",
			hitLine:  4,
			wantEnd:  4,
		},
		{
			name:     "rust doc comment and attribute",
			language: "rust",
			content:  "/// Returns a greeting.\n#[inline]\nfn greet() -> &'static str {\n    \"hello\"\n}\n",
			hitLine:  4,
			wantEnd:  5,
		},
		{
			name:     "shell comment",
			language: "shell",
			content:  "# Build the project.\nbuild() {\n  echo build\n}\n",
			hitLine:  3,
			wantEnd:  4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			segments := BuildSegments(test.content, test.language, map[int]bool{test.hitLine: true}, 20)
			assertLineRange(t, segments, 1, test.wantEnd)
		})
	}
}

func TestBuildSegmentsTreatsCommentMatchAsDeclarationMatch(t *testing.T) {
	content := "/** Fetches the user. */\nfunction fetchUser() {\n  return user;\n}\n"
	segments := BuildSegments(content, "javascript", map[int]bool{1: true}, 20)
	assertLineRange(t, segments, 1, 4)
}

func TestLeadingCommentAllowsOneBlankLine(t *testing.T) {
	content := "/** Fetches the user. */\n\nfunction fetchUser() {\n  return user;\n}\n"
	segments := BuildSegments(content, "javascript", map[int]bool{4: true}, 20)
	assertLineRange(t, segments, 1, 5)
}

func TestLeadingCommentDoesNotCrossTwoBlankLines(t *testing.T) {
	content := "/** Unrelated note. */\n\n\nfunction fetchUser() {\n  return user;\n}\n"
	segments := BuildSegments(content, "javascript", map[int]bool{5: true}, 20)
	assertLineRange(t, segments, 4, 6)
}

func TestTrailingCommentDoesNotAttachToNextDeclaration(t *testing.T) {
	content := "const value = 1; // not fetchUser documentation\nfunction fetchUser() {\n  return value;\n}\n"
	segments := BuildSegments(content, "javascript", map[int]bool{3: true}, 20)
	assertLineRange(t, segments, 2, 4)
}

func assertLineRange(t *testing.T, segments []Segment, wantStart, wantEnd int) {
	t.Helper()
	for _, segment := range segments {
		if segment.Kind == "lines" && segment.Start == wantStart && segment.End == wantEnd {
			return
		}
	}
	t.Fatalf("expected lines segment %d-%d, got %#v", wantStart, wantEnd, segments)
}
