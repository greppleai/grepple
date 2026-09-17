package navigation

import (
	"encoding/json"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestTextAndDocumentGraphBuildsMatch(t *testing.T) {
	sources := []TextSource{
		{Path: "pkg/helper.py", Text: "def finish():\n    pass\n"},
		{Path: "pkg/main.py", Text: "from helper import finish\ndef start():\n    finish()\n"},
	}
	documents := make([]DocumentSource, 0, len(sources))
	for _, source := range sources {
		document, err := parser.ParseDocument(parser.LanguageFor(source.Path), source.Text)
		if err != nil {
			t.Fatal(err)
		}
		defer document.Close()
		documents = append(documents, DocumentSource{Path: source.Path, Document: document})
	}
	fromText, textStats := BuildGraphFromTextSources(sources, BuildOptions{DisableCache: true})
	fromDocuments, documentStats := BuildGraphFromDocuments(documents, BuildOptions{DisableCache: true})
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
	left, _ := BuildGraphFromTextSources([]TextSource{first, second}, BuildOptions{DisableCache: true})
	right, _ := BuildGraphFromTextSources([]TextSource{second, first}, BuildOptions{DisableCache: true})
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("source order changed graph\nleft: %s\nright: %s", leftJSON, rightJSON)
	}
}
