package gritql

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestSupportedLanguagesReflectTargetAdapters(t *testing.T) {
	languages := SupportedLanguages()
	ids := make([]string, 0, len(languages))
	for _, language := range languages {
		ids = append(ids, language.ID)
		adapter, ok := targetLanguageByID(language.ID)
		if !ok || language.Grammar != adapter.grammar || language.TreeSitter != adapter.treeSitter {
			t.Fatalf("language capability does not match adapter: %#v", language)
		}
	}
	if want := []string{"c", "cpp", "csharp", "go", "java", "javascript", "kotlin", "python", "rust", "shell", "tsx", "typescript"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("language IDs=%#v, want %#v", ids, want)
	}
}

func TestGritQLCoversEveryTreeSitterLanguage(t *testing.T) {
	for _, language := range parser.SupportedLanguages() {
		if !language.Navigation {
			continue
		}
		if _, ok := targetLanguageByID(language.ID); !ok {
			t.Fatalf("Tree-sitter language %q has no GritQL adapter", language.ID)
		}
	}
}
