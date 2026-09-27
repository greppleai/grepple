package search

import (
	"strings"

	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// ConfigureSourcePolicy projects neutral repository policy into Search parameters.
func ConfigureSourcePolicy(params *Params, provider sourcedomain.Provider) error {
	if provider == nil {
		return nil
	}
	options, err := provider.ScopeOptions()
	if err != nil {
		return err
	}
	ApplySourcePolicy(params, options)
	return nil
}

// SourcePolicyConfigurer adapts a policy provider to Search loader callbacks.
func SourcePolicyConfigurer(provider sourcedomain.Provider) func(*Params) error {
	return func(params *Params) error { return ConfigureSourcePolicy(params, provider) }
}

// ApplySourcePolicy applies neutral source policy and reports explicit bypasses.
func ApplySourcePolicy(params *Params, options sourcedomain.Options) {
	if params == nil {
		return
	}
	params.ProductionOnly = options.ProductionOnly
	params.IgnoreRoot = options.Root()
	params.IgnorePaths = append([]string(nil), options.IgnorePaths...)
	explicit := append([]string(nil), params.Globs...)
	if separator := strings.LastIndex(params.At, ":"); separator > 0 {
		explicit = append(explicit, params.At[:separator])
	}
	sourcedomain.ReportExplicitBypasses(explicit, options)
}
