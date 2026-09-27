package api

import "github.com/greppleai/grepple/internal/wire"

const (
	// FeatureUnsupported is the public alias of the shared wire constant.
	FeatureUnsupported = wire.FeatureUnsupported
	// FeatureProduction is the public alias of the shared wire constant.
	FeatureProduction = wire.FeatureProduction
	// FeatureSpecialized is the public alias of the shared wire constant.
	FeatureSpecialized = wire.FeatureSpecialized
	// FeatureExperimental is the public alias of the shared wire constant.
	FeatureExperimental = wire.FeatureExperimental
)

// FeatureSupport is the public alias of the shared wire contract.
type FeatureSupport = wire.FeatureSupport

// NavigationFactCapabilities is the public alias of the shared wire contract.
type NavigationFactCapabilities = wire.NavigationFactCapabilities

// LanguageCapabilities is the public alias of the shared wire contract.
type LanguageCapabilities = wire.LanguageCapabilities
