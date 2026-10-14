package render

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
)

// Validated spans, not wire-provided labels, control what becomes a public
// annotation. In particular a capable remote peer cannot print arbitrary names.
// The parser owns the fixed public vocabulary used by both line search and
// local --at selection. Remote labels are checked against that same allowlist.

func validatedScopeRange(span *parser.ScopeLineRange, line int) *parser.ScopeLineRange {
	if span == nil || !parser.PublicScopeKind(span.Kind) || span.StartLine < 1 || span.EndLine < span.StartLine || line < span.StartLine || line > span.EndLine {
		return nil
	}
	return span
}

// scopeDisplayMatch never mutates the shared result. An unrequested annotation
// is not leaked through JSON even if a peer sent unsolicited optional fields.
func scopeDisplayMatch(match search.ResultMatch, annotate bool) search.ResultMatch {
	if !annotate {
		match.Nearest, match.Scope = nil, nil
		return match
	}
	match.Nearest = validatedScopeRange(match.Nearest, match.Line)
	match.Scope = validatedScopeRange(match.Scope, match.Line)
	return match
}

// validateAnnotatedSourceLines rejects malformed remote text before any human
// output. JSON still carries the literal source even when it contains newlines.
func validateAnnotatedSourceLines(results []search.FileResult) error {
	for _, result := range results {
		for _, match := range result.Matches {
			if strings.ContainsAny(normalizeRenderedAnchorLine(match.Text), "\r\n") {
				return fmt.Errorf("cannot render annotated match at %s:%d: embedded line break; use --json", result.Path, match.Line)
			}
		}
	}
	return nil
}

// decoratedMatchSource adds up to two distinct validated ranges to a source
// line. Human suffixes are display-only: use typed JSON to disambiguate source
// text that itself ends in an annotation-shaped string.
func decoratedMatchSource(match search.ResultMatch) string {
	source := normalizeRenderedAnchorLine(match.Text)
	var suffix strings.Builder
	var previous *parser.ScopeLineRange
	for _, span := range []*parser.ScopeLineRange{validatedScopeRange(match.Nearest, match.Line), validatedScopeRange(match.Scope, match.Line)} {
		if span == nil {
			continue
		}
		if previous != nil && span.StartLine == previous.StartLine && span.EndLine == previous.EndLine {
			continue
		}
		fmt.Fprintf(&suffix, " @%s:%d-%d", span.Kind, span.StartLine, span.EndLine)
		previous = span
	}
	return source + suffix.String()
}

// scopeDisplayResults returns copies only when a match needs filtering. The
// legacy response stays identical when all optional annotations are absent.
func scopeDisplayResults(results []search.FileResult, annotate bool) []search.FileResult {
	var sanitized []search.FileResult
	var copied []bool
	for i, result := range results {
		for j, match := range result.Matches {
			clean := scopeDisplayMatch(match, annotate)
			if clean.Nearest == match.Nearest && clean.Scope == match.Scope {
				continue
			}
			if sanitized == nil {
				sanitized = append([]search.FileResult(nil), results...)
				copied = make([]bool, len(results))
			}
			if !copied[i] {
				sanitized[i].Matches = append([]search.ResultMatch(nil), result.Matches...)
				copied[i] = true
			}
			sanitized[i].Matches[j] = clean
		}
	}
	if sanitized == nil {
		return results
	}
	return sanitized
}
