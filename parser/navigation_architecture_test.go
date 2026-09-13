package parser

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSharedParserEnginesContainNoLanguageSpecificBranches(t *testing.T) {
	languageIDs := make(map[string]bool, len(languageAdapters))
	for id := range languageAdapters {
		languageIDs[id] = true
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "language") || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		assertNavigationFileHasNoLanguageIDs(t, name, languageIDs)
	}
}

func assertNavigationFileHasNoLanguageIDs(t *testing.T, path string, languageIDs map[string]bool) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := ParseDocument("go", string(content))
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	_ = document.Read(func(view DocumentView) error {
		WalkNamedView(view.Root(), func(node ViewNode) {
			if node.Kind() != "interpreted_string_literal" && node.Kind() != "raw_string_literal" {
				return
			}
			value, err := strconv.Unquote(node.Text())
			if err == nil && languageIDs[value] {
				t.Errorf("%s contains language ID %q; move syntax policy into the owning language adapter", path, value)
			}
		})
		return nil
	})
}

func TestNavigationCollectorDependsOnAdaptersNotLanguageStrings(t *testing.T) {
	content, err := os.ReadFile("navigation.go")
	if err != nil {
		t.Fatal(err)
	}
	document, err := ParseDocument("go", string(content))
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	_ = document.Read(func(view DocumentView) error {
		WalkNamedView(view.Root(), func(node ViewNode) {
			if node.Kind() != "type_spec" || node.ChildByFieldName("name").Text() != "navigationCollector" {
				return
			}
			WalkNamedView(node.ChildByFieldName("type"), func(field ViewNode) {
				if field.Kind() == "field_identifier" && field.Text() == "language" {
					t.Error("navigationCollector must depend on adapters, not a language field")
				}
			})
		})
		return nil
	})
}

func TestEveryLanguageAdapterProvidesNavigationSemantics(t *testing.T) {
	for id, adapter := range languageAdapters {
		navigation := navigationAdapterForLanguage(id)
		if navigation == nil {
			t.Errorf("language adapter %q has no navigation semantics", id)
			continue
		}
		if navigation.Rules() != adapter.Rules() {
			t.Errorf("language adapter %q navigation uses a different rule set", id)
		}
	}
}
