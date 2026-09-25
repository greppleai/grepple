package archdaemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/parser"
)

// ResolveSelection identifies one symbol preview independent of presentation flags.
type ResolveSelection struct {
	Symbol       string   `json:"symbol"`
	Languages    []string `json:"languages,omitempty"`
	Visibilities []string `json:"visibilities,omitempty"`
}

const ResolveProjectionSchema = "grepple-daemon-resolve-projection-v1"

// ResolveProjection contains only the matching declaration facts, not the whole graph.
type ResolveProjection struct {
	Schema       string                         `json:"schema"`
	Sources      analysis.SourceSummary         `json:"sources"`
	Truncation   *analysis.Truncation           `json:"truncation,omitempty"`
	Declarations []parser.NavigationDeclaration `json:"declarations"`
}

func variantKey(base, kind string, selection any) (string, bool) {
	encoded, err := json.Marshal(selection)
	if err != nil || base == "" || kind == "" || string(encoded) == "null" {
		return "", false
	}
	digest := sha256.New()
	writeKeyPart(digest, base)
	writeKeyPart(digest, kind)
	writeKeyPart(digest, string(encoded))
	return hex.EncodeToString(digest.Sum(nil)), true
}

func selectedVariantKey(base string, payload request, kind string) (string, bool) {
	switch kind {
	case architectureVariant:
		return base, true
	case graphVariant:
		if payload.GraphQuery == nil || payload.GraphQuery.Direction == "" || payload.GraphQuery.Depth < 1 || payload.GraphQuery.Depth > 10 {
			return "", false
		}
		return variantKey(base, kind, payload.GraphQuery)
	case resolveVariant:
		if payload.Selection == nil || payload.Selection.Symbol == "" {
			return "", false
		}
		return variantKey(base, kind, payload.Selection)
	}
	return "", false
}

func keyForVariant(paths []string, maxFiles int, sources []analysis.Source, kind string, query any) (string, bool) {
	if _, ok := readDescriptor(); !ok {
		return "", false
	}
	root, err := os.Getwd()
	if err != nil {
		return "", false
	}
	base, ok := architectureFingerprint(root, paths, maxFiles, sources)
	if !ok {
		return "", false
	}
	return variantKey(base, kind, query)
}

// QueryGraph returns a previously published focused graph, without running analysis in the worker.
func QueryGraph(paths []string, maxFiles int, query analysis.GraphQuery) (analysis.GraphReport, bool) {
	root, err := os.Getwd()
	if err != nil {
		return analysis.GraphReport{}, false
	}
	body, ok := postDaemon("/graph", request{Root: root, Paths: paths, MaxFiles: maxFiles, GraphQuery: &query}, maxRequestBytes)
	if !ok {
		return analysis.GraphReport{}, false
	}
	var result response
	if json.Unmarshal(body, &result) != nil || result.Graph == nil || result.Graph.Schema != analysis.GraphSchema {
		return analysis.GraphReport{}, false
	}
	return *result.Graph, true
}

// KeyGraph fingerprints already-read sources plus the focused query.
func KeyGraph(paths []string, maxFiles int, sources []analysis.Source, query analysis.GraphQuery) (string, bool) {
	return keyForVariant(paths, maxFiles, sources, graphVariant, query)
}

// StoreGraph best-effort publishes a focused graph.
func StoreGraph(paths []string, maxFiles int, key string, query analysis.GraphQuery, graph analysis.GraphReport) bool {
	if key == "" || graph.Schema != analysis.GraphSchema {
		return false
	}
	root, err := os.Getwd()
	if err != nil {
		return false
	}
	_, ok := postDaemon("/store", request{Root: root, Paths: paths, MaxFiles: maxFiles, Kind: graphVariant, Key: key, GraphQuery: &query, Graph: &graph}, maxResponseBytes)
	return ok
}

// QueryResolve returns the small, source-linked symbol projection for one selector.
func QueryResolve(paths []string, maxFiles int, selection ResolveSelection) (ResolveProjection, bool) {
	root, err := os.Getwd()
	if err != nil {
		return ResolveProjection{}, false
	}
	body, ok := postDaemon("/resolve", request{Root: root, Paths: paths, MaxFiles: maxFiles, Selection: &selection}, maxRequestBytes)
	if !ok {
		return ResolveProjection{}, false
	}
	var result response
	if json.Unmarshal(body, &result) != nil || result.Projection == nil || result.Projection.Schema != ResolveProjectionSchema {
		return ResolveProjection{}, false
	}
	return *result.Projection, true
}

// KeyResolve fingerprints already-read sources plus the symbol/filter selector.
func KeyResolve(paths []string, maxFiles int, sources []analysis.Source, selection ResolveSelection) (string, bool) {
	return keyForVariant(paths, maxFiles, sources, resolveVariant, selection)
}

// StoreResolve best-effort publishes a small symbol projection.
func StoreResolve(paths []string, maxFiles int, key string, selection ResolveSelection, projection ResolveProjection) bool {
	if key == "" || projection.Schema != ResolveProjectionSchema {
		return false
	}
	root, err := os.Getwd()
	if err != nil {
		return false
	}
	_, ok := postDaemon("/store", request{Root: root, Paths: paths, MaxFiles: maxFiles, Kind: resolveVariant, Key: key, Selection: &selection, Projection: &projection}, maxResponseBytes)
	return ok
}
