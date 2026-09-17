package search

import "github.com/greppleai/grepple/parser"

type boundaryLanguagePolicy interface {
	standardLibraryImport(string) bool
	thirdPartyImport(string) bool
	publicTypeUsage(parser.NavigationDeclaration, string) bool
}

type baseBoundaryLanguagePolicy struct{}

func (baseBoundaryLanguagePolicy) standardLibraryImport(string) bool { return false }
func (baseBoundaryLanguagePolicy) thirdPartyImport(string) bool      { return false }
func (baseBoundaryLanguagePolicy) publicTypeUsage(parser.NavigationDeclaration, string) bool {
	return true
}

var defaultBoundaryLanguagePolicy boundaryLanguagePolicy = baseBoundaryLanguagePolicy{}

var boundaryLanguagePolicies = func() map[string]boundaryLanguagePolicy {
	ecma := ecmaBoundaryLanguagePolicy{}
	return map[string]boundaryLanguagePolicy{
		"go":         goBoundaryLanguagePolicy{},
		"javascript": ecma,
		"typescript": ecma,
		"tsx":        ecma,
		"java":       javaBoundaryLanguagePolicy{},
		"kotlin":     kotlinBoundaryLanguagePolicy{},
		"csharp":     cSharpBoundaryLanguagePolicy{},
	}
}()

func boundaryPolicyFor(language string) boundaryLanguagePolicy {
	if policy := boundaryLanguagePolicies[language]; policy != nil {
		return policy
	}
	return defaultBoundaryLanguagePolicy
}
