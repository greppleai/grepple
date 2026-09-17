package extract

import (
	"reflect"
	"strings"
	"testing"
)

func TestSupportedLanguagesAreStableAndDefensive(t *testing.T) {
	languages := SupportedLanguages()
	if len(languages) != 10 {
		t.Fatalf("SupportedLanguages() = %#v", languages)
	}
	if languages[0].ID != "go" || !reflect.DeepEqual(languages[0].Extensions, []string{".go"}) || !languages[0].FocusedStructure || !languages[0].FocusedFlow {
		t.Fatalf("Go adapter metadata = %#v", languages[0])
	}
	if languages[1].ID != "typescript" || !reflect.DeepEqual(languages[1].Extensions, []string{".ts", ".mts", ".cts", ".tsx"}) || !languages[1].FocusedStructure || !languages[1].FocusedFlow {
		t.Fatalf("TypeScript adapter metadata = %#v", languages[1])
	}
	if languages[2].ID != "javascript" || !reflect.DeepEqual(languages[2].Extensions, []string{".js", ".jsx"}) || !languages[2].FocusedStructure || !languages[2].FocusedFlow {
		t.Fatalf("JavaScript adapter metadata = %#v", languages[2])
	}
	if languages[3].ID != "python" || !reflect.DeepEqual(languages[3].Extensions, []string{".py", ".pyi", ".pyw"}) || !languages[3].FocusedStructure || !languages[3].FocusedFlow {
		t.Fatalf("Python adapter metadata = %#v", languages[3])
	}
	for index, expected := range []struct {
		id         string
		extensions []string
	}{{"java", []string{".java"}}, {"kotlin", []string{".kt", ".kts"}}, {"csharp", []string{".cs"}}, {"rust", []string{".rs"}}, {"c", []string{".c", ".h"}}, {"cpp", []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx"}}} {
		language := languages[index+4]
		if language.ID != expected.id || !reflect.DeepEqual(language.Extensions, expected.extensions) || !language.FocusedStructure || !language.FocusedFlow {
			t.Fatalf("class-model adapter metadata = %#v", language)
		}
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
		"module.mts": "typescript", "module.cts": "typescript", "app.js": "javascript", "view.jsx": "javascript",
		"main.py": "python", "types.pyi": "python", "gui.pyw": "python", "Main.java": "java", "build.kt": "kotlin", "script.kts": "kotlin",
		"main.rs": "rust", "main.c": "c", "api.h": "c", "service.cpp": "cpp", "service.hpp": "cpp",
	} {
		language, ok := LanguageForPath(path)
		if !ok || language.ID != expected {
			t.Errorf("LanguageForPath(%q) = %#v, %v", path, language, ok)
		}
	}
	if _, ok := LanguageForPath("main.swift"); ok {
		t.Fatal("unsupported extension was assigned an adapter")
	}
}

func TestAnalysisRejectsSourcesWithoutAnAdapter(t *testing.T) {
	_, err := Analyze([]Source{{Path: "main.swift", Text: "func main() {}"}})
	if err == nil || !strings.Contains(err.Error(), "unsupported source language") || !strings.Contains(err.Error(), "rust") {
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
	if _, err := ParseClassDiagram("classDiagram\n class Item\n <<python>> Item\n"); err != nil {
		t.Fatalf("registered Python class language: %v", err)
	}
	if _, err := ParseFlowchart("flowchart TD\n item[Item]\n %% grepple:language item python\n"); err != nil {
		t.Fatalf("registered Python flow language: %v", err)
	}
	if _, err := ParseClassDiagram("classDiagram\n class Item\n <<rust>> Item\n"); err != nil {
		t.Fatalf("registered Rust class language: %v", err)
	}
	if _, err := ParseClassDiagram("classDiagram\n class Item\n <<swift>> Item\n"); err == nil || !strings.Contains(err.Error(), "unsupported stereotype") {
		t.Fatalf("unregistered class language: %v", err)
	}
}
