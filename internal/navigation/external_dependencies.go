package navigation

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/internal/dependency"
)

var externalDependencyResolvers = dependency.DefaultRegistry()

type dependencyContext struct {
	language     string
	resolver     dependency.Resolver
	dependencies []dependency.Evidence
	applicable   bool
}

// qualifyLocalDependencies adds exact local manifest evidence to unresolved
// dependency references. References without an exact version remain unchanged.
func qualifyLocalDependencies[T ExternalDependencyResult](results []T, workingDirectory string) error {
	return qualifyExternalDependencies(results, workingDirectory, false)
}

// qualifyRepositoryExternalDependencies qualifies server-side results whose paths
// are relative to an indexed repository root.
func qualifyRepositoryExternalDependencies[T ExternalDependencyResult](results []T, repositoryRoot string) error {
	return qualifyExternalDependencies(results, repositoryRoot, true)
}

func qualifyExternalDependencies[T ExternalDependencyResult](results []T, workingDirectory string, includeRepositoryResults bool) error {
	if workingDirectory == "" {
		workingDirectory, _ = os.Getwd()
	}
	contexts := make(map[string]dependencyContext)
	for resultIndex := range results {
		if err := qualifyExternalDependencyResult(results[resultIndex].ExternalDependencyData(), workingDirectory, includeRepositoryResults, contexts); err != nil {
			return err
		}
	}
	return nil
}

func qualifyExternalDependencyResult(result ExternalDependencyData, workingDirectory string, includeRepositoryResults bool, contexts map[string]dependencyContext) error {
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

func firstExternalReferenceLanguage(symbols []RelatedSymbol) string {
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

func qualifyDependencySymbolsForContext(symbols []RelatedSymbol, context dependencyContext) {
	for index := range symbols {
		if reference := symbols[index].External; reference != nil && reference.Language == context.language {
			applyDependencyMatch(reference, context.resolver.Match(reference.ImportPath, context.dependencies))
		}
		qualifyDependencySymbolsForContext(symbols[index].Related, context)
	}
}

func applyDependencyMatch(reference *ExternalReference, match dependency.Match) {
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
	reference.Candidates = make([]ExternalDependencyCandidate, 0, len(match.Candidates))
	for _, candidate := range match.Candidates {
		reference.Candidates = append(reference.Candidates, ExternalDependencyCandidate{
			Ecosystem: candidate.Ecosystem,
			Module:    candidate.Module,
			Version:   candidate.Version,
			Integrity: candidate.Integrity,
			Source:    candidate.Source,
		})
	}
}

// externalDependencyReferences returns deterministic exact references eligible
// for a server artifact lookup.
func externalDependencyReferences[T ExternalDependencyResult](results []T) []ExternalReference {
	byID := make(map[string]ExternalReference)
	var collect func([]RelatedSymbol)
	collect = func(symbols []RelatedSymbol) {
		for _, symbol := range symbols {
			if reference := symbol.External; reference != nil && reference.ID != "" && externalReferenceQualified(*reference) {
				byID[reference.ID] = *reference
			}
			collect(symbol.Related)
		}
	}
	for _, result := range results {
		collect(result.ExternalDependencyData().Related)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	references := make([]ExternalReference, 0, len(ids))
	for _, id := range ids {
		references = append(references, byID[id])
	}
	return references
}

func externalReferenceQualified(reference ExternalReference) bool {
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

// applyExternalDependencyResolution replaces unresolved references with exact
// symbols returned by a navigation artifact server.
func applyExternalDependencyResolution[T interface {
	ExternalDependencyResult
	WithExternalDependencyRelated([]RelatedSymbol) T
}](results []T, response ResolveResponse) []T {
	resolved := make(map[string][]RelatedSymbol, len(response.Results))
	for _, result := range response.Results {
		resolved[result.ID] = append(resolved[result.ID], result.Symbols...)
	}
	for index := range results {
		results[index] = results[index].WithExternalDependencyRelated(applyExternalDependencySymbols(results[index].ExternalDependencyData().Related, resolved))
	}
	return results
}

func applyExternalDependencySymbols(symbols []RelatedSymbol, resolved map[string][]RelatedSymbol) []RelatedSymbol {
	result := make([]RelatedSymbol, 0, len(symbols))
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
