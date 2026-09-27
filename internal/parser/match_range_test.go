package parser

import "testing"

func TestStructuralLineRangesFindDeclarationsAndControlFlow(t *testing.T) {
	tests := []struct {
		name, language, content string
		lines                   map[int]bool
		want                    map[int]StructuralLineRange
	}{
		{
			name:     "go function and if",
			language: "go",
			content:  "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n}\nconst value = 1\n",
			lines:    map[int]bool{2: true, 3: true, 7: true},
			want: map[int]StructuralLineRange{
				2: {StartLine: 2, EndLine: 6},
				3: {StartLine: 3, EndLine: 5},
			},
		},
		{
			name:     "typescript function",
			language: "typescript",
			content:  "export function run() {\n  return work();\n}\n",
			lines:    map[int]bool{1: true},
			want:     map[int]StructuralLineRange{1: {StartLine: 1, EndLine: 3}},
		},
		{
			name:     "python branch",
			language: "python",
			content:  "def run():\n    if ready:\n        work()\n    return 1\n",
			lines:    map[int]bool{1: true, 2: true},
			want: map[int]StructuralLineRange{
				1: {StartLine: 1, EndLine: 4},
				2: {StartLine: 2, EndLine: 3},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := StructuralLineRanges(test.content, test.language, test.lines)
			if len(got) != len(test.want) {
				t.Fatalf("ranges = %#v, want %#v", got, test.want)
			}
			for line, want := range test.want {
				if got[line] != want {
					t.Fatalf("line %d range = %#v, want %#v", line, got[line], want)
				}
			}
		})
	}
}

func TestEnclosingLineRangesFindNearestNestedConstruct(t *testing.T) {
	goContent := "package sample\nfunc Run() {\n\tif ready {\n\t\twork()\n\t}\n\tfinish()\n}\n"
	got := EnclosingLineRanges(goContent, "go", map[int]bool{4: true, 6: true})
	want := map[int]StructuralLineRange{
		4: {StartLine: 3, EndLine: 5},
		6: {StartLine: 2, EndLine: 7},
	}
	for line, expected := range want {
		if got[line] != expected {
			t.Fatalf("line %d range = %#v, want %#v", line, got[line], expected)
		}
	}

	pythonContent := "def run():\n    if ready:\n        work()\n    finish()\n"
	pythonRange := EnclosingLineRanges(pythonContent, "python", map[int]bool{3: true})[3]
	if pythonRange != (StructuralLineRange{StartLine: 2, EndLine: 3}) {
		t.Fatalf("python range = %#v", pythonRange)
	}
}

func TestStructuralLineRangesCoverEveryTreeSitterLanguage(t *testing.T) {
	tests := []struct {
		language string
		content  string
		line     int
		endLine  int
	}{
		{language: "c", content: "int run(void) {\n  return 1;\n}\n", line: 1, endLine: 3},
		{language: "cpp", content: "int run() {\n  return 1;\n}\n", line: 1, endLine: 3},
		{language: "csharp", content: "class App {\n  int Run() {\n    return 1;\n  }\n}\n", line: 2, endLine: 4},
		{language: "go", content: "package sample\nfunc run() {\n  work()\n}\n", line: 2, endLine: 4},
		{language: "java", content: "class App {\n  int run() {\n    return 1;\n  }\n}\n", line: 2, endLine: 4},
		{language: "javascript", content: "function run() {\n  return 1;\n}\n", line: 1, endLine: 3},
		{language: "kotlin", content: "fun run() {\n  println(1)\n}\n", line: 1, endLine: 3},
		{language: "python", content: "def run():\n    return 1\n", line: 1, endLine: 2},
		{language: "rust", content: "fn run() {\n  work();\n}\n", line: 1, endLine: 3},
		{language: "shell", content: "run() {\n  echo yes\n}\n", line: 1, endLine: 3},
		{language: "typescript", content: "function run(): number {\n  return 1;\n}\n", line: 1, endLine: 3},
		{language: "tsx", content: "function Run(): JSX.Element {\n  return <div />;\n}\n", line: 1, endLine: 3},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			got := StructuralLineRanges(test.content, test.language, map[int]bool{test.line: true})[test.line]
			if got.StartLine != test.line || got.EndLine != test.endLine {
				t.Fatalf("range = %#v, want %d-%d", got, test.line, test.endLine)
			}
			bodyLine := test.line + 1
			enclosing := EnclosingLineRanges(test.content, test.language, map[int]bool{bodyLine: true})[bodyLine]
			if enclosing.StartLine != test.line || enclosing.EndLine != test.endLine {
				t.Fatalf("enclosing range = %#v, want %d-%d", enclosing, test.line, test.endLine)
			}
		})
	}
}

func TestStructuralLineRangesGracefullySkipsUnsupportedContent(t *testing.T) {
	if got := StructuralLineRanges("# heading\ntext\n", "markdown", map[int]bool{1: true}); len(got) != 0 {
		t.Fatalf("unexpected ranges: %#v", got)
	}
}
