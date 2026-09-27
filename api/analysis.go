package api

import "github.com/greppleai/grepple/internal/wire"

const (
	// AnalysisGraph is the public alias of the shared wire constant.
	AnalysisGraph = wire.AnalysisGraph
	// AnalysisArchitecture is the public alias of the shared wire constant.
	AnalysisArchitecture = wire.AnalysisArchitecture
	// AnalysisBoundaries is the public alias of the shared wire constant.
	AnalysisBoundaries = wire.AnalysisBoundaries
	// AnalysisResponsibilities is the public alias of the shared wire constant.
	AnalysisResponsibilities = wire.AnalysisResponsibilities
)

// AnalysisOperation is the public alias of the shared wire contract.
type AnalysisOperation = wire.AnalysisOperation

// AnalysisRequest is the public alias of the shared wire contract.
type AnalysisRequest = wire.AnalysisRequest

// GraphQueryRequest is the public alias of the shared wire contract.
type GraphQueryRequest = wire.GraphQueryRequest

// AnalysisResponse is the public alias of the shared wire contract.
type AnalysisResponse = wire.AnalysisResponse
