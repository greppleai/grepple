package navigation

// ResultSegment is one structural source segment attached to a related symbol.
type ResultSegment struct {
	Kind  string `json:"kind"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

// ArtifactIdentity pins a related declaration to one immutable dependency source artifact.
type ArtifactIdentity struct {
	Ecosystem  string `json:"ecosystem"`
	Module     string `json:"module"`
	Version    string `json:"version"`
	RefKind    string `json:"refKind,omitempty"`
	Integrity  string `json:"integrity,omitempty"`
	Source     string `json:"source,omitempty"`
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// ExternalDependencyCandidate is one exact manifest or lockfile identity that
// may provide an externally referenced symbol.
type ExternalDependencyCandidate struct {
	Ecosystem string `json:"ecosystem"`
	Module    string `json:"module"`
	Version   string `json:"version"`
	Integrity string `json:"integrity,omitempty"`
	Source    string `json:"source,omitempty"`
}

// ExternalReference retains syntax evidence needed to resolve one dependency symbol.
type ExternalReference struct {
	ID              string                        `json:"id"`
	Language        string                        `json:"language"`
	ImportPath      string                        `json:"importPath"`
	Package         string                        `json:"package,omitempty"`
	Symbol          string                        `json:"symbol"`
	ConsumerPackage string                        `json:"consumerPackage,omitempty"`
	ReceiverType    string                        `json:"receiverType,omitempty"`
	Kind            string                        `json:"kind"`
	Module          string                        `json:"module,omitempty"`
	Version         string                        `json:"version,omitempty"`
	Integrity       string                        `json:"integrity,omitempty"`
	Source          string                        `json:"source,omitempty"`
	Candidates      []ExternalDependencyCandidate `json:"candidates,omitempty"`
}

// RelatedSymbol points between matched code and a local or artifact-qualified declaration.
type RelatedSymbol struct {
	Name           string             `json:"name"`
	Path           string             `json:"path"`
	Kind           string             `json:"kind"`
	Direction      string             `json:"direction"`
	Start          int                `json:"start"`
	End            int                `json:"end"`
	CallLine       int                `json:"callLine"`
	Confidence     string             `json:"confidence"`
	Role           string             `json:"role,omitempty"`
	External       *ExternalReference `json:"external,omitempty"`
	Artifact       *ArtifactIdentity  `json:"artifact,omitempty"`
	Segments       []ResultSegment    `json:"segments,omitempty"`
	Related        []RelatedSymbol    `json:"related,omitempty"`
	OmittedCallers int                `json:"omittedCallers,omitempty"`
	OmittedCallees int                `json:"omittedCallees,omitempty"`
	OmittedTypes   int                `json:"omittedTypes,omitempty"`
}

// ResolveRequest batches exact dependency references for artifact lookup.
type ResolveRequest struct {
	References []ExternalReference `json:"references"`
}

// ResolveResult preserves correlation between a reference and declaration candidates.
type ResolveResult struct {
	ID      string          `json:"id"`
	Symbols []RelatedSymbol `json:"symbols,omitempty"`
}

// ResolveResponse is returned by an immutable navigation artifact index.
type ResolveResponse struct {
	Results []ResolveResult `json:"results"`
}

// ExternalDependencyResult exposes the search-result data needed by dependency qualification.
type ExternalDependencyResult interface {
	ExternalDependencyData() ExternalDependencyData
}

// ExternalDependencyData is an invocation-neutral view of one result.
type ExternalDependencyData struct {
	Path, Repo, Language string
	Related              []RelatedSymbol
}
