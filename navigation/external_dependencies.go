package navigation

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/dependency"
)

var externalDependencyResolvers = dependency.DefaultRegistry()

type dependencyContext struct {
	language     string
	resolver     dependency.Resolver
	dependencies []dependency.Evidence
	applicable   bool
}

// QualifyExternalDependencies adds exact local manifest evidence to unresolved
// dependency references. References without an exact version remain unchanged.
func QualifyExternalDependencies(results []api.FileResult, workingDirectory string) error {
	return qualifyExternalDependencies(results, workingDirectory, false)
}

// QualifyRepositoryExternalDependencies qualifies server-side results whose paths
// are relative to an indexed repository root.
func QualifyRepositoryExternalDependencies(results []api.FileResult, repositoryRoot string) error {
	return qualifyExternalDependencies(results, repositoryRoot, true)
}

func qualifyExternalDependencies(results []api.FileResult, workingDirectory string, includeRepositoryResults bool) error {
	if workingDirectory == "" {
		workingDirectory, _ = os.Getwd()
	}
	contexts := make(map[string]dependencyContext)
	for resultIndex := range results {
		if err := qualifyExternalDependencyResult(&results[resultIndex], workingDirectory, includeRepositoryResults, contexts); err != nil {
			return err
		}
	}
	return nil
}

func qualifyExternalDependencyResult(result *api.FileResult, workingDirectory string, includeRepositoryResults bool, contexts map[string]dependencyContext) error {
	if result.Repo != "" && !includeRepositoryResults {
		return nil
	}
	path := result.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDirectory, path)
	}
	language := result.Language
	if language == "" {
		language = firstExternalReferenceLanguage(result.Related)
	}
	for _, project := range externalDependencyResolvers.Projects(language, filepath.Dir(path)) {
		key := language + "\x00" + project.Key()
		context, exists := contexts[key]
		if !exists {
			resolution, err := project.Resolve()
			if err != nil {
				return err
			}
			context = dependencyContext{language: language, resolver: project.Resolver, dependencies: resolution.Dependencies, applicable: resolution.Applicable}
			contexts[key] = context
		}
		if !context.applicable {
			continue
		}
		qualifyDependencySymbolsForContext(result.Related, context)
		return nil
	}
	return nil
}

func firstExternalReferenceLanguage(symbols []api.RelatedSymbol) string {
	for _, symbol := range symbols {
		if symbol.External != nil && symbol.External.Language != "" {
			return symbol.External.Language
		}
		if language := firstExternalReferenceLanguage(symbol.Related); language != "" {
			return language
		}
	}
	return ""
}

func qualifyDependencySymbolsForContext(symbols []api.RelatedSymbol, context dependencyContext) {
	for index := range symbols {
		if reference := symbols[index].External; reference != nil && reference.Language == context.language {
			applyDependencyMatch(reference, context.resolver.Match(reference.ImportPath, context.dependencies))
		}
		qualifyDependencySymbolsForContext(symbols[index].Related, context)
	}
}

func applyDependencyMatch(reference *api.ExternalNavigationReference, match dependency.Match) {
	if match.Exact != nil {
		reference.Module = match.Exact.Module
		reference.Version = match.Exact.Version
		reference.Integrity = match.Exact.Integrity
		reference.Source = match.Exact.Source
		reference.Package = match.Package
		return
	}
	if len(match.Candidates) == 0 {
		return
	}
	reference.Candidates = make([]api.ExternalDependencyCandidate, 0, len(match.Candidates))
	for _, candidate := range match.Candidates {
		reference.Candidates = append(reference.Candidates, api.ExternalDependencyCandidate{
			Ecosystem: candidate.Ecosystem,
			Module:    candidate.Module,
			Version:   candidate.Version,
			Integrity: candidate.Integrity,
			Source:    candidate.Source,
		})
	}
}

// ExternalDependencyReferences returns deterministic exact references eligible
// for a server artifact lookup.
func ExternalDependencyReferences(results []api.FileResult) []api.ExternalNavigationReference {
	byID := make(map[string]api.ExternalNavigationReference)
	var collect func([]api.RelatedSymbol)
	collect = func(symbols []api.RelatedSymbol) {
		for _, symbol := range symbols {
			if reference := symbol.External; reference != nil && reference.ID != "" && externalReferenceQualified(*reference) {
				byID[reference.ID] = *reference
			}
			collect(symbol.Related)
		}
	}
	for _, result := range results {
		collect(result.Related)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	references := make([]api.ExternalNavigationReference, 0, len(ids))
	for _, id := range ids {
		references = append(references, byID[id])
	}
	return references
}

func externalReferenceQualified(reference api.ExternalNavigationReference) bool {
	if reference.Module != "" && reference.Version != "" {
		return true
	}
	for _, candidate := range reference.Candidates {
		if candidate.Ecosystem != "" && candidate.Module != "" && candidate.Version != "" {
			return true
		}
	}
	return false
}

// ApplyExternalDependencyResolution replaces unresolved references with exact
// symbols returned by a navigation artifact server.
func ApplyExternalDependencyResolution(results []api.FileResult, response api.NavigationResolveResponse) []api.FileResult {
	resolved := make(map[string][]api.RelatedSymbol, len(response.Results))
	for _, result := range response.Results {
		resolved[result.ID] = append(resolved[result.ID], result.Symbols...)
	}
	for index := range results {
		results[index].Related = applyExternalDependencySymbols(results[index].Related, resolved)
	}
	return results
}

func applyExternalDependencySymbols(symbols []api.RelatedSymbol, resolved map[string][]api.RelatedSymbol) []api.RelatedSymbol {
	result := make([]api.RelatedSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		if symbol.External != nil && len(resolved[symbol.External.ID]) > 0 {
			for _, replacement := range resolved[symbol.External.ID] {
				replacement.CallLine = symbol.CallLine
				replacement.Direction = symbol.Direction
				replacement.Role = symbol.Role
				result = append(result, replacement)
			}
			continue
		}
		symbol.Related = applyExternalDependencySymbols(symbol.Related, resolved)
		result = append(result, symbol)
	}
	return result
}
