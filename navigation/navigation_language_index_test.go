package navigation

import (
	"reflect"
	"sort"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestNavigationLanguageIndexRegistryCoversSupportedFamilies(t *testing.T) {
	graph := parser.NavigationGraph{}
	corpus := &navigationCorpus{
		contents: map[string]string{},
		exports:  map[string][]navigationExport{},
		graph:    graph,
	}
	indexes, _ := newLanguageNavigationIndexes(corpus, nil)
	if !reflect.DeepEqual(sortedNavigationFamilies(indexes), sortedParserNavigationFamilies()) {
		t.Fatalf("resolver families=%v parser navigation families=%v", sortedNavigationFamilies(indexes), sortedParserNavigationFamilies())
	}
	for family, index := range indexes {
		if index == nil {
			t.Fatalf("language family %q has a nil navigation index", family)
		}
	}
}

func sortedNavigationFamilies(indexes map[string]languageNavigationIndex) []string {
	families := make([]string, 0, len(indexes))
	for family := range indexes {
		families = append(families, family)
	}
	sort.Strings(families)
	return families
}

func sortedParserNavigationFamilies() []string {
	seen := make(map[string]bool)
	for _, capability := range parser.SupportedLanguages() {
		if capability.Navigation {
			seen[navigationLanguageFamily(capability.ID)] = true
		}
	}
	families := make([]string, 0, len(seen))
	for family := range seen {
		families = append(families, family)
	}
	sort.Strings(families)
	return families
}
