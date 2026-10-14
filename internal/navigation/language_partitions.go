package navigation

import (
	"sort"

	"github.com/greppleai/grepple/internal/parser"
)

// LanguagePartition is an independent graph storage unit. Languages with exact
// shared identity, call edges or file context remain together. Foreign field
// context is copied selectively without loading its language's adjacency core.
// This preserves current query semantics without guessing cross-language edges.
type LanguagePartition struct {
	Languages []string
	Graph     parser.NavigationGraph
}

type languagePartitionLinks struct {
	parent     map[string]string
	paths, ids map[string]string
}

func (links *languagePartitionLinks) find(language string) string {
	parent, ok := links.parent[language]
	if !ok {
		links.parent[language] = language
		return language
	}
	if parent != language {
		links.parent[language] = links.find(parent)
	}
	return links.parent[language]
}
func (links *languagePartitionLinks) join(a, b string) {
	a, b = links.find(a), links.find(b)
	if a != b {
		if a > b {
			a, b = b, a
		}
		links.parent[b] = a
	}
}
func (links *languagePartitionLinks) link(index map[string]string, key, language string) {
	links.find(language)
	if previous, ok := index[key]; ok {
		links.join(previous, language)
	} else {
		index[key] = language
	}
}
func (links *languagePartitionLinks) source(language, path string) {
	links.link(links.paths, path, language)
}
func (links *languagePartitionLinks) caller(language, id string) {
	if other, ok := links.ids[id]; ok {
		links.join(language, other)
	}
}
func partitionLanguageLinks(graph parser.NavigationGraph) *languagePartitionLinks {
	links := &languagePartitionLinks{parent: map[string]string{}, paths: map[string]string{}, ids: map[string]string{}}
	for _, d := range graph.Declarations {
		links.source(d.Language, d.Path)
		links.link(links.ids, d.ID, d.Language)
	}
	for _, f := range graph.Fields {
		links.source(f.Language, f.Path)
	}
	for _, c := range graph.Calls {
		links.source(c.Language, c.Path)
		links.caller(c.Language, c.CallerID)
		links.caller(c.Language, c.TargetID)
		for _, id := range c.CandidateTargetIDs {
			links.caller(c.Language, id)
		}
	}
	for _, x := range graph.TypeUsages {
		links.source(x.Language, x.Path)
		links.caller(x.Language, x.CallerID)
	}
	for _, x := range graph.MemberAccesses {
		links.source(x.Language, x.Path)
		links.caller(x.Language, x.CallerID)
	}
	for _, x := range graph.TypeDeclarations {
		links.source(x.Language, x.Path)
	}
	for _, x := range graph.Imports {
		links.source(x.Language, x.Path)
	}
	for _, x := range graph.Exports {
		links.source(x.Language, x.Path)
	}
	return links
}
func partitionNavigationLanguages(graph parser.NavigationGraph) []LanguagePartition {
	links := partitionLanguageLinks(graph)
	groups := map[string][]string{}
	for language := range links.parent {
		root := links.find(language)
		groups[root] = append(groups[root], language)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]LanguagePartition, 0, len(keys))
	for _, key := range keys {
		languages := groups[key]
		sort.Strings(languages)
		parts = append(parts, LanguagePartition{Languages: languages, Graph: partitionLanguageFacts(graph, links, key)})
	}
	return parts
}
func partitionRows[T any](rows []T, language func(T) string, links *languagePartitionLinks, key string) []T {
	if rows == nil {
		return nil
	}
	result := make([]T, 0)
	for _, row := range rows {
		if links.find(language(row)) == key {
			result = append(result, row)
		}
	}
	return result
}
func partitionLanguageFacts(graph parser.NavigationGraph, links *languagePartitionLinks, key string) parser.NavigationGraph {
	declarations := partitionRows(graph.Declarations, func(x parser.NavigationDeclaration) string { return x.Language }, links, key)
	calls := partitionRows(graph.Calls, func(x parser.NavigationCall) string { return x.Language }, links, key)
	owners := navigationQueryFieldOwners(declarations, calls)
	fields := partitionMatchingRows(graph.Fields, func(x parser.NavigationField) bool {
		return links.find(x.Language) == key || owners[terminalSymbolName(x.OwnerType)]
	})
	paths := navigationQueryFiles(declarations, calls, fields)
	return parser.NavigationGraph{
		RepositoryRoots: append([]string(nil), graph.RepositoryRoots...), Declarations: declarations, Calls: calls, Fields: fields,
		TypeDeclarations: partitionMatchingRows(graph.TypeDeclarations, func(x parser.NavigationTypeDeclaration) bool { return links.find(x.Language) == key || paths[x.Path] }),
		Imports:          partitionMatchingRows(graph.Imports, func(x parser.NavigationImport) bool { return links.find(x.Language) == key || paths[x.Path] }),
		Exports:          partitionMatchingRows(graph.Exports, func(x parser.NavigationExport) bool { return links.find(x.Language) == key || paths[x.Path] }),
		TypeUsages:       partitionRows(graph.TypeUsages, func(x parser.NavigationTypeUsage) string { return x.Language }, links, key),
		MemberAccesses:   partitionRows(graph.MemberAccesses, func(x parser.NavigationMemberAccess) string { return x.Language }, links, key),
	}
}
func partitionMatchingRows[T any](rows []T, include func(T) bool) []T {
	if rows == nil {
		return nil
	}
	result := make([]T, 0)
	for _, row := range rows {
		if include(row) {
			result = append(result, row)
		}
	}
	return result
}
