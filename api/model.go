// Package api exposes Grepple HTTP contracts and backend-facing services.
package api

import "github.com/greppleai/grepple/internal/wire"

const (
	// RuleModeCount is the public alias of the shared wire constant.
	RuleModeCount = wire.RuleModeCount
	// RuleModeFiles is the public alias of the shared wire constant.
	RuleModeFiles = wire.RuleModeFiles
	// RuleEngineText is the public alias of the shared wire constant.
	RuleEngineText = wire.RuleEngineText
	// RuleEngineGritQL is the public alias of the shared wire constant.
	RuleEngineGritQL = wire.RuleEngineGritQL
)

// SearchRequest is the public alias of the shared wire contract.
type SearchRequest = wire.SearchRequest

// ResultMatch is the public alias of the shared wire contract.
type ResultMatch = wire.ResultMatch

// ResultSegment is the public alias of the shared wire contract.
type ResultSegment = wire.ResultSegment

// ContextLine is the public alias of the shared wire contract.
type ContextLine = wire.ContextLine

// NavigationArtifactIdentity is the public alias of the shared wire contract.
type NavigationArtifactIdentity = wire.NavigationArtifactIdentity

// ExternalDependencyCandidate is the public alias of the shared wire contract.
type ExternalDependencyCandidate = wire.ExternalDependencyCandidate

// ExternalNavigationReference is the public alias of the shared wire contract.
type ExternalNavigationReference = wire.ExternalNavigationReference

// NavigationResolveRequest is the public alias of the shared wire contract.
type NavigationResolveRequest = wire.NavigationResolveRequest

// NavigationResolveResult is the public alias of the shared wire contract.
type NavigationResolveResult = wire.NavigationResolveResult

// NavigationResolveResponse is the public alias of the shared wire contract.
type NavigationResolveResponse = wire.NavigationResolveResponse

// RelatedSymbol is the public alias of the shared wire contract.
type RelatedSymbol = wire.RelatedSymbol

// LineRangeResult is the public alias of the shared wire contract.
type LineRangeResult = wire.LineRangeResult

// FileResult is the public alias of the shared wire contract.
type FileResult = wire.FileResult

// RepoCount is the public alias of the shared wire contract.
type RepoCount = wire.RepoCount

// SearchResponse is the public alias of the shared wire contract.
type SearchResponse = wire.SearchResponse

// SourceAnalysis is the public alias of the shared wire contract.
type SourceAnalysis = wire.SourceAnalysis

// RepoListEntry is the public alias of the shared wire contract.
type RepoListEntry = wire.RepoListEntry

// RepositoryIndexConfig is the public alias of the shared wire contract.
type RepositoryIndexConfig = wire.RepositoryIndexConfig

// RepositoryIndexTarget is the public alias of the shared wire contract.
type RepositoryIndexTarget = wire.RepositoryIndexTarget

// ReposResponse is the public alias of the shared wire contract.
type ReposResponse = wire.ReposResponse

// TreeEntry is the public alias of the shared wire contract.
type TreeEntry = wire.TreeEntry

// TreeResponse is the public alias of the shared wire contract.
type TreeResponse = wire.TreeResponse

// Rule is the public alias of the shared wire contract.
type Rule = wire.Rule

// RuleRepoResult is the public alias of the shared wire contract.
type RuleRepoResult = wire.RuleRepoResult

// RuleResults is the public alias of the shared wire contract.
type RuleResults = wire.RuleResults

// RuleSet is the public alias of the shared wire contract.
type RuleSet = wire.RuleSet
