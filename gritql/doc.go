// Package gritql implements Grepple's native, read-only gritql-go-v1
// structural-search compiler, matcher, evaluator, and bounded scanner.
//
// Programs are immutable and safe for concurrent evaluation. The package accepts
// Go source only, applies explicit resource limits, and performs no networking,
// process execution, source writes, or fallback-engine dispatch.
package gritql
