package boundaryanalysis

import "strings"

type ecmaBoundaryLanguagePolicy struct{ baseBoundaryLanguagePolicy }

func (ecmaBoundaryLanguagePolicy) standardLibraryImport(path string) bool {
	return strings.HasPrefix(path, "node:")
}

type javaBoundaryLanguagePolicy struct{ baseBoundaryLanguagePolicy }

func (javaBoundaryLanguagePolicy) standardLibraryImport(path string) bool {
	return path == "java" || strings.HasPrefix(path, "java.") || path == "javax" || strings.HasPrefix(path, "javax.")
}

type kotlinBoundaryLanguagePolicy struct{ baseBoundaryLanguagePolicy }

func (kotlinBoundaryLanguagePolicy) standardLibraryImport(path string) bool {
	return path == "kotlin" || strings.HasPrefix(path, "kotlin.")
}

type cSharpBoundaryLanguagePolicy struct{ baseBoundaryLanguagePolicy }

func (cSharpBoundaryLanguagePolicy) standardLibraryImport(path string) bool {
	return path == "System" || strings.HasPrefix(path, "System.")
}
