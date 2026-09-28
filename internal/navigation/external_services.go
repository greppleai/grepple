package navigation

// ExternalDependencyService qualifies source-linked dependency evidence and
// collects exact references for one result representation.
type ExternalDependencyService[T ExternalDependencyResult] interface {
	QualifyLocal([]T, string) error
	QualifyRepository([]T, string) error
	References([]T) []ExternalReference
}

type externalDependencyService[T ExternalDependencyResult] struct{}

// NewExternalDependencyService keeps the result type intact across qualification.
func NewExternalDependencyService[T ExternalDependencyResult]() ExternalDependencyService[T] {
	return externalDependencyService[T]{}
}

func (externalDependencyService[T]) QualifyLocal(results []T, directory string) error {
	return qualifyLocalDependencies(results, directory)
}

func (externalDependencyService[T]) QualifyRepository(results []T, root string) error {
	return qualifyRepositoryExternalDependencies(results, root)
}

func (externalDependencyService[T]) References(results []T) []ExternalReference {
	return externalDependencyReferences(results)
}

// ResolvableExternalResult is a result that can retain updated related symbols.
type ResolvableExternalResult[T any] interface {
	ExternalDependencyResult
	WithExternalDependencyRelated([]RelatedSymbol) T
}

// ExternalResolutionService merges server-resolved symbols without changing the result type.
type ExternalResolutionService[T ResolvableExternalResult[T]] interface {
	Apply([]T, ResolveResponse) []T
}

type externalResolutionService[T ResolvableExternalResult[T]] struct{}

// NewExternalResolutionService builds a type-preserving resolution capability.
func NewExternalResolutionService[T ResolvableExternalResult[T]]() ExternalResolutionService[T] {
	return externalResolutionService[T]{}
}

func (externalResolutionService[T]) Apply(results []T, response ResolveResponse) []T {
	return applyExternalDependencyResolution(results, response)
}
