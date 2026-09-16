package api

// FeatureSupport identifies whether a language feature is available and how it
// is implemented.
type FeatureSupport string

const (
	// FeatureUnsupported means the feature is not implemented for the language.
	FeatureUnsupported FeatureSupport = "unsupported"
	// FeatureProduction means the feature uses the normal production path.
	FeatureProduction FeatureSupport = "production"
	// FeatureSpecialized means the feature uses a production format-specific path.
	FeatureSpecialized FeatureSupport = "specialized"
	// FeatureExperimental means the feature is available only experimentally.
	FeatureExperimental FeatureSupport = "experimental"
)

// LanguageCapabilities is one row in the cross-feature language support matrix.
type LanguageCapabilities struct {
	Language              string         `json:"language"`
	Extensions            []string       `json:"extensions"`
	TextGrep              FeatureSupport `json:"textGrep"`
	StructuralGrep        FeatureSupport `json:"structuralGrep"`
	Outline               FeatureSupport `json:"outline"`
	Navigation            FeatureSupport `json:"navigation"`
	FocusedStructure      FeatureSupport `json:"focusedStructure"`
	FocusedFlow           FeatureSupport `json:"focusedFlow"`
	GritQL                FeatureSupport `json:"gritql"`
	DirectoryArchitecture FeatureSupport `json:"directoryArchitecture"`
	ImportRelations       FeatureSupport `json:"importRelations"`
	Entrypoints           FeatureSupport `json:"entrypoints"`
}
