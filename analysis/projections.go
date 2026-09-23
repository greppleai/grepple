package analysis

import (
	"sort"
	"strings"
)

// ArchitectureResolveReport is the structured architecture symbol projection.
type ArchitectureResolveReport struct {
	Schema  string               `json:"schema"`
	Symbol  string               `json:"symbol"`
	Sources SourceSummary        `json:"sources"`
	Matches []ArchitectureSymbol `json:"matches"`
}

// ArchitectureWhyReport is the structured architecture relation projection.
type ArchitectureWhyReport struct {
	Schema   string                         `json:"schema"`
	From     string                         `json:"from"`
	To       string                         `json:"to"`
	Relation string                         `json:"relation"`
	Sources  SourceSummary                  `json:"sources"`
	Evidence []ArchitectureRelationEvidence `json:"evidence"`
}

// ResolveArchitectureSymbols finds exact and terminal symbol matches.
func ResolveArchitectureSymbols(symbols []ArchitectureSymbol, query string) []ArchitectureSymbol {
	matches := make([]ArchitectureSymbol, 0)
	for _, symbol := range symbols {
		terminal := symbol.Name
		if separator := strings.LastIndexAny(terminal, ".:#"); separator >= 0 {
			terminal = terminal[separator+1:]
		}
		if symbol.Name == query || terminal == query {
			matches = append(matches, symbol)
		}
	}
	return matches
}

// ArchitectureRelationEvidenceFor selects evidence between two directories.
func ArchitectureRelationEvidenceFor(relations []ArchitectureRelation, from, to string) []ArchitectureRelationEvidence {
	from, to = cleanDirectory(from), cleanDirectory(to)
	evidence := []ArchitectureRelationEvidence{}
	for _, relation := range relations {
		if relation.From == from && relation.To == to {
			evidence = append(evidence, relation.Evidence...)
		}
	}
	return evidence
}

// CleanArchitectureDirectory normalizes a directory identity.
func CleanArchitectureDirectory(value string) string { return cleanDirectory(value) }

// ArchitectureEvidenceRelation summarizes deterministic evidence relation kinds.
func ArchitectureEvidenceRelation(evidence []ArchitectureRelationEvidence) string {
	seen := make(map[string]bool)
	kinds := make([]string, 0)
	for _, item := range evidence {
		if !seen[item.Kind] {
			seen[item.Kind] = true
			kinds = append(kinds, item.Kind)
		}
	}
	sort.Strings(kinds)
	if len(kinds) == 0 {
		return "none"
	}
	return strings.Join(kinds, ",")
}
