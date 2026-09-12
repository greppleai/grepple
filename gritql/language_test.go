package gritql

import (
	"reflect"
	"testing"
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
	if want := []string{"go", "javascript", "tsx", "typescript"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("language IDs=%#v, want %#v", ids, want)
	}
}
