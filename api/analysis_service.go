package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/boundaryanalysis"
)

// AnalysisSource is one selected repository-relative file and its source bytes.
// ReadError keeps an unreadable selected file in the completeness accounting.
type AnalysisSource struct {
	Path      string
	Content   []byte
	ReadError error
}

// BoundaryPolicy is a validated, opaque repository policy for an analysis run.
// A nil policy represents the absence of repository rules.
type BoundaryPolicy interface {
	isBoundaryPolicy()
}

type decodedBoundaryPolicy struct {
	value boundaryanalysis.BoundaryPolicy
}

func (decodedBoundaryPolicy) isBoundaryPolicy() {}

// DecodeBoundaryPolicy rejects unknown fields, trailing data, and invalid policy.
func DecodeBoundaryPolicy(content []byte) (BoundaryPolicy, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var policy boundaryanalysis.BoundaryPolicy
	if err := decoder.Decode(&policy); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON content")
	}
	if err := boundaryanalysis.ValidateBoundaryPolicy(policy); err != nil {
		return nil, err
	}
	return decodedBoundaryPolicy{value: policy}, nil
}

// AnalyzeSources evaluates one request over an already selected, complete
// source snapshot. It returns the versioned operation document and whether
// every selected source was analyzed without truncation or read/parse failure.
func AnalyzeSources(request AnalysisRequest, sources []AnalysisSource, policy BoundaryPolicy, policyPath string) (json.RawMessage, bool, error) {
	input := make([]analysis.Source, len(sources))
	for i, source := range sources {
		input[i] = analysis.Source{Path: source.Path, Content: source.Content, ReadError: source.ReadError}
	}
	universe, err := analysis.NewUniverse(input, request.MaxFiles)
	if err != nil {
		return nil, false, err
	}
	defer universe.Close()

	var result any
	switch request.Operation {
	case AnalysisGraph:
		result, err = analysis.BuildGraph(universe, analysisGraphQuery(request.Graph))
	case AnalysisArchitecture:
		result = analysis.BuildArchitecture(universe)
	case AnalysisBoundaries:
		var rules boundaryanalysis.BoundaryPolicy
		if policy != nil {
			decoded, ok := policy.(decodedBoundaryPolicy)
			if !ok {
				return nil, false, fmt.Errorf("invalid boundary policy implementation")
			}
			rules = decoded.value
		}
		result, err = analysis.BuildBoundaries(universe, request.Paths, request.MinOccurrences, rules, policyPath)
	case AnalysisResponsibilities:
		result = analysis.BuildResponsibilities(universe)
	default:
		return nil, false, fmt.Errorf("unsupported analysis operation %q", request.Operation)
	}
	if err != nil {
		return nil, false, err
	}
	content, err := json.Marshal(result)
	if err != nil {
		return nil, false, fmt.Errorf("encode %s analysis: %w", request.Operation, err)
	}
	return content, universe.Truncation() == nil && universe.Summary().Failed == 0, nil
}

func analysisGraphQuery(request *GraphQueryRequest) *analysis.GraphQuery {
	if request == nil {
		return nil
	}
	return &analysis.GraphQuery{
		Direction: request.Direction, Depth: request.Depth, Symbol: request.Symbol,
		At: request.At, Package: request.Package, Module: request.Module,
		RootPath: request.RootPath, Languages: request.Languages,
		Confidences: request.Confidences, Visibilities: request.Visibilities,
	}
}
