package extract

import (
	"reflect"
	"strings"
	"testing"
)

func TestSupportedLanguagesAreStableAndDefensive(t *testing.T) {
	languages := SupportedLanguages()
	if len(languages) != 2 {
		t.Fatalf("SupportedLanguages() = %#v", languages)
	}
	if languages[0].ID != "go" || !reflect.DeepEqual(languages[0].Extensions, []string{".go"}) {
		t.Fatalf("Go adapter metadata = %#v", languages[0])
	}
	if languages[1].ID != "typescript" || !reflect.DeepEqual(languages[1].Extensions, []string{".ts", ".tsx", ".mts", ".cts"}) {
		t.Fatalf("TypeScript adapter metadata = %#v", languages[1])
	}
	languages[0].Extensions[0] = ".changed"
	fresh := SupportedLanguages()
	if fresh[0].Extensions[0] != ".go" {
		t.Fatalf("caller mutated registered language metadata: %#v", fresh[0])
	}
}

func TestLanguageAdaptersAreCompleteAndNonOverlapping(t *testing.T) {
	identifiers, extensions := map[string]bool{}, map[string]bool{}
	for _, definition := range registeredLanguages() {
		if definition.info.ID == "" || identifiers[definition.info.ID] {
			t.Fatalf("invalid or duplicate language ID %q", definition.info.ID)
		}
		identifiers[definition.info.ID] = true
		if definition.acceptsSource == nil || definition.newAnalysis == nil || definition.nearestProjectRoot == nil || definition.sourceScope == nil || definition.normalizeType == nil || definition.generateStructure == nil || definition.generateFlow == nil || definition.validFlowEdge == nil {
			t.Fatalf("language adapter %q is incomplete", definition.info.ID)
		}
		for _, extension := range definition.info.Extensions {
			if extension == "" || extensions[extension] {
				t.Fatalf("invalid or duplicate language extension %q", extension)
			}
			extensions[extension] = true
		}
	}
}

func TestLanguageForPathUsesAdapterExtensions(t *testing.T) {
	for path, expected := range map[string]string{
		"main.go": "go", "view.ts": "typescript", "view.tsx": "typescript",
		"module.mts": "typescript", "module.cts": "typescript",
	} {
		language, ok := LanguageForPath(path)
		if !ok || language.ID != expected {
			t.Errorf("LanguageForPath(%q) = %#v, %v", path, language, ok)
		}
	}
	if _, ok := LanguageForPath("main.py"); ok {
		t.Fatal("unsupported extension was assigned an adapter")
	}
}

func TestAnalysisRejectsSourcesWithoutAnAdapter(t *testing.T) {
	_, err := Analyze([]Source{{Path: "main.py", Text: "def main(): pass"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported source language") || !strings.Contains(err.Error(), "go or typescript") {
		t.Fatalf("unsupported source error = %v", err)
	}
}

func TestLanguageMetadataUsesRegisteredAdapterIDs(t *testing.T) {
	if _, err := ParseClassDiagram("classDiagram\n class Item\n <<go>> Item\n"); err != nil {
		t.Fatalf("registered class language: %v", err)
	}
	if _, err := ParseFlowchart("flowchart TD\n item[Item]\n %% grepple:language item typescript\n"); err != nil {
		t.Fatalf("registered flow language: %v", err)
	}
	if _, err := ParseClassDiagram("classDiagram\n class Item\n <<python>> Item\n"); err == nil || !strings.Contains(err.Error(), "unsupported stereotype") {
		t.Fatalf("unregistered class language: %v", err)
	}
	if _, err := ParseFlowchart("flowchart TD\n item[Item]\n %% grepple:language item python\n"); err == nil || !strings.Contains(err.Error(), "unsupported language") {
		t.Fatalf("unregistered flow language: %v", err)
	}
}
