// Package gritqlapi adapts native structural-engine results to shared wire DTOs.
package gritqlapi

import (
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/parser"
)

// Response converts an immutable scan result without changing its normalized order.
func Response(result gritql.ScanResult) wire.GritResponse {
	findings := result.Findings()
	diagnostics := result.Diagnostics()
	truncations := result.Truncations()
	response := wire.GritResponse{
		Metadata:    metadata(result.Metadata()),
		Findings:    make([]wire.GritFinding, 0, len(findings)),
		Diagnostics: make([]wire.GritDiagnostic, 0, len(diagnostics)),
		Truncations: make([]wire.GritTruncation, 0, len(truncations)),
		ShardErrors: []string{},
		Statistics:  statistics(result.Stats()),
	}
	for _, finding := range findings {
		response.Findings = append(response.Findings, convertFinding(finding))
	}
	for _, diagnostic := range diagnostics {
		response.Diagnostics = append(response.Diagnostics, convertDiagnostic(diagnostic))
	}
	for _, truncation := range truncations {
		response.Truncations = append(response.Truncations, wire.GritTruncation{Reason: truncation.Reason, Limit: truncation.Limit, Skipped: truncation.Skipped})
	}
	return response
}

func convertFinding(finding gritql.Finding) wire.GritFinding {
	bindings := finding.Bindings()
	result := wire.GritFinding{
		Path: finding.Path(), Language: finding.Language(), Range: convertRange(finding.Range()), Text: finding.Text(),
		PatternID: finding.PatternID(), Message: finding.Message(), Bindings: make([]wire.GritBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		result.Bindings = append(result.Bindings, convertBinding(binding))
	}
	return result
}

func convertBinding(binding gritql.FindingBinding) wire.GritBinding {
	ranges := binding.Ranges()
	structural := binding.Structural()
	result := wire.GritBinding{
		Name: binding.Name(), Kind: binding.Kind().String(), Range: convertRange(binding.Range()),
		Ranges: make([]wire.GritRange, 0, len(ranges)), Structural: make([]wire.GritStructuralNode, 0, len(structural)),
	}
	for _, bindingRange := range ranges {
		result.Ranges = append(result.Ranges, convertRange(bindingRange))
	}
	for _, node := range structural {
		result.Structural = append(result.Structural, convertStructuralNode(node))
	}
	return result
}

func convertStructuralNode(node gritql.StructuralNode) wire.GritStructuralNode {
	children := node.Children()
	result := wire.GritStructuralNode{NodeKind: node.NodeKind(), TokenKind: node.TokenKind(), Lexeme: node.Lexeme()}
	if len(children) > 0 {
		result.Children = make([]wire.GritStructuralNode, 0, len(children))
		for _, child := range children {
			result.Children = append(result.Children, convertStructuralNode(child))
		}
	}
	return result
}

func convertDiagnostic(diagnostic gritql.Diagnostic) wire.GritDiagnostic {
	result := wire.GritDiagnostic{Code: diagnostic.Code(), Class: diagnostic.Class(), Severity: diagnostic.Severity(), Message: diagnostic.Message()}
	if value, ok := diagnostic.PatternID(); ok {
		result.PatternID = &value
	}
	if value, ok := diagnostic.Path(); ok {
		result.Path = &value
	}
	if value, ok := diagnostic.Range(); ok {
		converted := convertRange(value)
		result.Range = &converted
	}
	return result
}

func convertRange(sourceRange parser.Range) wire.GritRange {
	return wire.GritRange{
		StartByte: sourceRange.StartByte, EndByte: sourceRange.EndByte,
		Start: wire.GritPosition{Line: sourceRange.Start.Line, Column: sourceRange.Start.Column},
		End:   wire.GritPosition{Line: sourceRange.End.Line, Column: sourceRange.End.Column},
	}
}

func metadata(source gritql.EvaluationMetadata) wire.GritMetadata {
	limits := source.Limits
	return wire.GritMetadata{Compatibility: source.Contract, Language: source.Language, Grammar: source.Grammar, GoGrammar: source.GoGrammar, Limits: wire.GritEffectiveLimits{
		PatternBytes: limits.PatternBytes, RegexBytes: limits.RegexBytes, RegexInstructions: limits.RegexInstructions,
		ParseDepth: limits.ParseDepth, SourceBytes: limits.SourceBytes, Candidates: limits.Candidates,
		ASTSteps: limits.ASTSteps, Findings: limits.Findings, FileTimeMillis: limits.FileTimeMillis,
		BatchTimeMillis: limits.BatchTimeMillis, MemoryBytes: int64(limits.MemoryBytes),
	}}
}

func statistics(source gritql.ScanStats) wire.GritStatistics {
	return wire.GritStatistics{
		Candidates: source.Candidates, Eligible: source.Eligible, Evaluated: source.Evaluated, BytesRead: source.BytesRead,
		SkippedLanguage: source.SkippedLanguage, SkippedGlob: source.SkippedGlob, SkippedBinary: source.SkippedBinary, SkippedAnchor: source.SkippedAnchor,
	}
}
