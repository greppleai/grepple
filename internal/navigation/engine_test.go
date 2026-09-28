package navigation

import (
	"encoding/json"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestTextAndDocumentGraphBuildsMatch(t *testing.T) {
	sources := []TextSource{
		{Path: "pkg/helper.py", Text: "def finish():\n    pass\n"},
		{Path: "pkg/main.py", Text: "from helper import finish\ndef start():\n    finish()\n"},
	}
	documents := make([]DocumentSource, 0, len(sources))
	for _, source := range sources {
		document, err := parser.NewParser().Parse(parser.NewParser().LanguageFor(source.Path), source.Text)
		if err != nil {
			t.Fatal(err)
		}
		defer document.Close()
		documents = append(documents, DocumentSource{Path: source.Path, Document: document})
	}
	builder := NewGraphEngine(BuildOptions{DisableCache: true})
	textAnalysis, textStats := builder.BuildTextSources(sources)
	documentAnalysis, documentStats := builder.BuildDocuments(documents)
	fromText, fromDocuments := textAnalysis.Graph(), documentAnalysis.Graph()
	textJSON, err := json.Marshal(fromText)
	if err != nil {
		t.Fatal(err)
	}
	documentJSON, err := json.Marshal(fromDocuments)
	if err != nil {
		t.Fatal(err)
	}
	if string(textJSON) != string(documentJSON) {
		t.Fatalf("text and document graphs differ\ntext: %s\ndocuments: %s", textJSON, documentJSON)
	}
	if textStats != documentStats || textStats.Parsed != len(sources) {
		t.Fatalf("stats differ: text=%+v documents=%+v", textStats, documentStats)
	}
}

func TestTextGraphBuildIsSourceOrderDeterministic(t *testing.T) {
	first := TextSource{Path: "src/a.ts", Text: "export function run(): void {}\n"}
	second := TextSource{Path: "src/b.ts", Text: "import { run } from './a';\nexport function start(): void { run(); }\n"}
	builder := NewGraphEngine(BuildOptions{DisableCache: true})
	leftAnalysis, _ := builder.BuildTextSources([]TextSource{first, second})
	rightAnalysis, _ := builder.BuildTextSources([]TextSource{second, first})
	left, right := leftAnalysis.Graph(), rightAnalysis.Graph()
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("source order changed graph\nleft: %s\nright: %s", leftJSON, rightJSON)
	}
}
