// Package gritql implements Grepple's native, read-only structural-search
// compiler, matcher, evaluator, and bounded scanner.
//
// Programs are immutable and safe for concurrent evaluation. Target-language
// syntax is isolated behind adapters for Go, JavaScript/JSX, TypeScript, TSX, and Python. The package
// applies explicit resource limits and performs no networking, process execution,
// source writes, or fallback-engine dispatch.
package gritql
