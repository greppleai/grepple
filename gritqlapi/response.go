// Package gritqlapi adapts native structural-engine results to dependency-free API DTOs.
package gritqlapi

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/gritql"
	"github.com/greppleai/grepple/parser"
)

// Response converts an immutable scan result without changing its normalized order.
func Response(result gritql.ScanResult) api.GritResponse {
	findings := result.Findings()
	diagnostics := result.Diagnostics()
	truncations := result.Truncations()
	response := api.GritResponse{
		Metadata:    metadata(result.Metadata()),
		Findings:    make([]api.GritFinding, 0, len(findings)),
		Diagnostics: make([]api.GritDiagnostic, 0, len(diagnostics)),
		Truncations: make([]api.GritTruncation, 0, len(truncations)),
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
		response.Truncations = append(response.Truncations, api.GritTruncation{Reason: truncation.Reason, Limit: truncation.Limit, Skipped: truncation.Skipped})
	}
	return response
}

func convertFinding(finding gritql.Finding) api.GritFinding {
	bindings := finding.Bindings()
	result := api.GritFinding{
		Path: finding.Path(), Language: finding.Language(), Range: convertRange(finding.Range()), Text: finding.Text(),
		PatternID: finding.PatternID(), Message: finding.Message(), Bindings: make([]api.GritBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		result.Bindings = append(result.Bindings, convertBinding(binding))
	}
	return result
}

func convertBinding(binding gritql.FindingBinding) api.GritBinding {
	ranges := binding.Ranges()
	structural := binding.Structural()
	result := api.GritBinding{
		Name: binding.Name(), Kind: binding.Kind().String(), Range: convertRange(binding.Range()),
		Ranges: make([]api.GritRange, 0, len(ranges)), Structural: make([]api.GritStructuralNode, 0, len(structural)),
	}
	for _, bindingRange := range ranges {
		result.Ranges = append(result.Ranges, convertRange(bindingRange))
	}
	for _, node := range structural {
		result.Structural = append(result.Structural, convertStructuralNode(node))
	}
	return result
}

func convertStructuralNode(node gritql.StructuralNode) api.GritStructuralNode {
	children := node.Children()
	result := api.GritStructuralNode{NodeKind: node.NodeKind(), TokenKind: node.TokenKind(), Lexeme: node.Lexeme()}
	if len(children) > 0 {
		result.Children = make([]api.GritStructuralNode, 0, len(children))
		for _, child := range children {
			result.Children = append(result.Children, convertStructuralNode(child))
		}
	}
	return result
}

func convertDiagnostic(diagnostic gritql.Diagnostic) api.GritDiagnostic {
	result := api.GritDiagnostic{Code: diagnostic.Code(), Class: diagnostic.Class(), Severity: diagnostic.Severity(), Message: diagnostic.Message()}
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

func convertRange(sourceRange parser.Range) api.GritRange {
	return api.GritRange{
		StartByte: sourceRange.StartByte, EndByte: sourceRange.EndByte,
		Start: api.GritPosition{Line: sourceRange.Start.Line, Column: sourceRange.Start.Column},
		End:   api.GritPosition{Line: sourceRange.End.Line, Column: sourceRange.End.Column},
	}
}

func metadata(source gritql.EvaluationMetadata) api.GritMetadata {
	limits := source.Limits
	return api.GritMetadata{Compatibility: source.Contract, Language: source.Language, Grammar: source.Grammar, GoGrammar: source.GoGrammar, Limits: api.GritEffectiveLimits{
		PatternBytes: limits.PatternBytes, RegexBytes: limits.RegexBytes, RegexInstructions: limits.RegexInstructions,
		ParseDepth: limits.ParseDepth, SourceBytes: limits.SourceBytes, Candidates: limits.Candidates,
		ASTSteps: limits.ASTSteps, Findings: limits.Findings, FileTimeMillis: limits.FileTimeMillis,
		BatchTimeMillis: limits.BatchTimeMillis, MemoryBytes: int64(limits.MemoryBytes),
	}}
}

func statistics(source gritql.ScanStats) api.GritStatistics {
	return api.GritStatistics{
		Candidates: source.Candidates, Eligible: source.Eligible, Evaluated: source.Evaluated, BytesRead: source.BytesRead,
		SkippedLanguage: source.SkippedLanguage, SkippedGlob: source.SkippedGlob, SkippedBinary: source.SkippedBinary,
	}
}
