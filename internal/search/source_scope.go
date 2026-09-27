package search

import sourcedomain "github.com/greppleai/grepple/internal/sources"

// SourceScopeOptions is retained as a compatibility alias for catalog options.
type SourceScopeOptions = sourcedomain.InspectionOptions

// SourcePathDecision is retained as a compatibility alias for catalog decisions.
type SourcePathDecision = sourcedomain.Decision

// InspectSourceScope delegates source selection inspection to the neutral catalog.
func InspectSourceScope(inputs []string, options SourceScopeOptions) ([]SourcePathDecision, error) {
	return sourcedomain.Inspect(inputs, options)
}
