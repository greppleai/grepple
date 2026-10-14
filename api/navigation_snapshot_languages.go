package api

import (
	"github.com/greppleai/grepple/internal/parser"
	"sort"
)

// SearchBatchLanguages reports only the selected navigation-capable result rows.
func SearchBatchLanguages(batch SearchBatch) []string {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	service := parser.NewParser()
	for _, match := range value.matches {
		if capabilities, ok := service.CapabilitiesForLanguage(match.Language); ok && capabilities.Navigation {
			seen[match.Language] = true
		}
	}
	result := make([]string, 0, len(seen))
	for language := range seen {
		result = append(result, language)
	}
	sort.Strings(result)
	return result
}
