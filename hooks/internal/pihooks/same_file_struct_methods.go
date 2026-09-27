// Package pihooks contains repository-specific checks used by the pi hooks.
package pihooks

import (
	"context"
	"path/filepath"

	"github.com/greppleai/grepple/internal/hookruntime"
)

const sameFileStructMethodsRule = "same-file-struct-methods"

// Diagnostic mirrors revive's JSON shape for the Pi Stop feedback pipeline.
type Diagnostic struct {
	Severity        string             `json:"Severity"`
	Failure         string             `json:"Failure"`
	RuleName        string             `json:"RuleName"`
	Category        string             `json:"Category"`
	Position        DiagnosticPosition `json:"Position"`
	Confidence      float64            `json:"Confidence"`
	ReplacementLine string             `json:"ReplacementLine"`
}

// DiagnosticPosition is the source range attached to a Diagnostic.
type DiagnosticPosition struct {
	Start SourcePosition `json:"Start"`
	End   SourcePosition `json:"End"`
}

// SourcePosition mirrors the position fields in a revive diagnostic.
type SourcePosition struct {
	Filename string `json:"Filename"`
	Offset   int    `json:"Offset"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

// AnalyzeRepository adapts the repository-owned relational GritQL rule to Pi's
// revive-shaped diagnostic protocol. Rule selection and joining live in YAML
// and the shared GritQL engine; this package no longer parses Go declarations.
func AnalyzeRepository(start string) ([]Diagnostic, error) {
	root, err := hookruntime.RepositoryRoot(start)
	if err != nil {
		return nil, err
	}
	findings, err := hookruntime.CheckRelation(context.Background(), root, sameFileStructMethodsRule)
	if err != nil {
		return nil, err
	}
	diagnostics := make([]Diagnostic, 0, len(findings))
	for _, item := range findings {
		filename := filepath.Join(root, filepath.FromSlash(item.Path))
		diagnostics = append(diagnostics, Diagnostic{
			Severity: item.Severity, Failure: item.Message,
			RuleName: sameFileStructMethodsRule, Category: "layout",
			Position: DiagnosticPosition{
				Start: SourcePosition{Filename: filename, Offset: 0, Line: item.Line, Column: 1},
				End:   SourcePosition{Filename: filename, Offset: 1, Line: item.Line, Column: 2},
			},
			Confidence: 1, ReplacementLine: "",
		})
	}
	return diagnostics, nil
}

func analyzeRepository(root string) ([]Diagnostic, error) { return AnalyzeRepository(root) }
