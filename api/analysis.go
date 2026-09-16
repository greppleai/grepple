package api

import "encoding/json"

// AnalysisOperation selects one complete-source, graph-backed repository analysis.
type AnalysisOperation string

const (
	// AnalysisGraph builds or queries the normalized navigation graph.
	AnalysisGraph AnalysisOperation = "graph"
	// AnalysisArchitecture builds a language-neutral directory architecture report.
	AnalysisArchitecture AnalysisOperation = "architecture"
	// AnalysisBoundaries finds graph-backed workflow and type boundary evidence.
	AnalysisBoundaries AnalysisOperation = "boundaries"
	// AnalysisResponsibilities summarizes directory ownership and relation participation.
	AnalysisResponsibilities AnalysisOperation = "responsibilities"
)

// AnalysisRequest asks an indexed shard to analyze exactly one repository checkout.
// Repository is an exact indexed OWNER/REPO[@REF] selector and paths are relative to it.
type AnalysisRequest struct {
	Operation      AnalysisOperation  `json:"operation"`
	Repository     string             `json:"repository"`
	Paths          []string           `json:"paths,omitempty"`
	MaxFiles       int                `json:"maxFiles,omitempty"`
	MinOccurrences int                `json:"minOccurrences,omitempty"`
	ProductionOnly bool               `json:"productionOnly,omitempty"`
	NoConfigIgnore bool               `json:"noConfigIgnore,omitempty"`
	NoRepoConfig   bool               `json:"noRepoConfig,omitempty"`
	Policy         string             `json:"policy,omitempty"`
	Graph          *GraphQueryRequest `json:"graph,omitempty"`
}

// GraphQueryRequest optionally projects a bounded traversal from the complete graph.
type GraphQueryRequest struct {
	Direction    string   `json:"direction"`
	Depth        int      `json:"depth,omitempty"`
	Symbol       string   `json:"symbol,omitempty"`
	At           string   `json:"at,omitempty"`
	Package      string   `json:"package,omitempty"`
	Module       string   `json:"module,omitempty"`
	RootPath     string   `json:"rootPath,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	Confidences  []string `json:"confidences,omitempty"`
	Visibilities []string `json:"visibilities,omitempty"`
}

// AnalysisResponse preserves exact checkout identity and an operation-specific,
// versioned result document. Found is false on shards that do not own Repository.
type AnalysisResponse struct {
	Schema      string            `json:"schema"`
	Operation   AnalysisOperation `json:"operation"`
	Repository  string            `json:"repository"`
	Commit      string            `json:"commit,omitempty"`
	Found       bool              `json:"found"`
	Complete    bool              `json:"complete"`
	Result      json.RawMessage   `json:"result,omitempty"`
	Notices     []string          `json:"notices,omitempty"`
	ShardErrors []string          `json:"shardErrors,omitempty"`
}
