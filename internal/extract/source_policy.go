package extract

import sourcedomain "github.com/greppleai/grepple/internal/sources"

// LoadSourcesWithPolicy projects neutral repository policy into Extract discovery.
func LoadSourcesWithPolicy(roots []string, options sourcedomain.Options) ([]Source, error) {
	sourcedomain.ReportExplicitBypasses(roots, options)
	return LoadSourcesWithOptions(roots, DiscoveryOptions{
		IgnoreRoot: options.Root(), IgnorePaths: append([]string(nil), options.IgnorePaths...), ProductionOnly: options.ProductionOnly,
	})
}
