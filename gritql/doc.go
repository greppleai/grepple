// Package gritql implements Grepple's native, read-only structural-search
// compiler, matcher, evaluator, and bounded scanner.
//
// Programs are immutable and safe for concurrent evaluation. Target-language
// syntax is isolated behind adapters for every parser-backed Tree-sitter language. The package
// applies explicit resource limits and performs no networking, process execution,
// source writes, or fallback-engine dispatch.
package gritql
