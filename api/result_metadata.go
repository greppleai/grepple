package api

// ResultMetadata gives every bounded Grepple result the same execution envelope.
// Domain-specific statistics and truncation records remain authoritative details.
type ResultMetadata struct {
	Scope       ResultScope        `json:"scope"`
	Order       string             `json:"order"`
	Page        ResultPage         `json:"page"`
	Limits      ResultLimits       `json:"limits"`
	Omitted     ResultOmissions    `json:"omitted"`
	Diagnostics []ResultDiagnostic `json:"diagnostics"`
	NextCommand string             `json:"nextCommand,omitempty"`
}

// ResultScope identifies the selected source universe.
type ResultScope struct {
	Mode                 string   `json:"mode"`
	Paths                []string `json:"paths"`
	ExcludedPaths        []string `json:"excludedPaths"`
	Repositories         []string `json:"repositories"`
	ExcludedRepositories []string `json:"excludedRepositories"`
	Languages            []string `json:"languages"`
}

// ResultPage describes the returned window. Total is absent when a distributed
// source cannot prove the complete result count.
type ResultPage struct {
	Skip     int  `json:"skip"`
	Limit    int  `json:"limit"`
	Returned int  `json:"returned"`
	Total    *int `json:"total,omitempty"`
	Complete bool `json:"complete"`
}

// ResultLimits publishes user-facing source, segment, and byte caps. Zero means
// unlimited for the corresponding local operation.
type ResultLimits struct {
	MaxFiles         int   `json:"maxFiles"`
	MaxOutputBytes   int   `json:"maxOutputBytes"`
	MaxSourceBytes   int   `json:"maxSourceBytes"`
	MaxTotalBytes    int64 `json:"maxTotalBytes"`
	JSONByteUncapped bool  `json:"jsonByteUncapped"`
}

// ResultOmissions normalizes known omitted totals. Unknown omissions remain zero
// and are explained by Page.Complete or diagnostics rather than estimated.
type ResultOmissions struct {
	Files    int   `json:"files"`
	Segments int   `json:"segments"`
	Sources  int   `json:"sources"`
	Findings int   `json:"findings"`
	Bytes    int64 `json:"bytes"`
}

// ResultDiagnostic is a stable summary suitable for every command family.
type ResultDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
