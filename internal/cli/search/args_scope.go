package search

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

// scopeSelectorFromArgs accepts only the second token of --at PATH @KIND:LINE[-END].
// Numeric-looking PATH:LINE remains reserved for legacy navigation.
func scopeSelectorFromArgs(values *Args) (*parser.ScopeSelector, error) {
	if values.At == "" || values.Query == "" || len(values.Globs) != 0 || !strings.HasPrefix(values.Query, "@") || legacyAtLocation(values.At) {
		return nil, nil
	}
	token := values.Query
	kind, lines, found := strings.Cut(strings.TrimPrefix(token, "@"), ":")
	if !found || kind == "" || lines == "" {
		return nil, fmt.Errorf("invalid --at selector %q: expected @KIND:LINE or @KIND:START-END", token)
	}
	if !parser.PublicScopeKind(kind) {
		return nil, fmt.Errorf("invalid --at selector %q: unknown scope kind %q", token, kind)
	}
	startText, endText, exact := strings.Cut(lines, "-")
	if !exact {
		endText = startText
	}
	start, okStart := positiveSelectorLine(startText)
	end, okEnd := positiveSelectorLine(endText)
	if !okStart || !okEnd || end < start {
		return nil, fmt.Errorf("invalid --at selector %q: expected positive ascending line bounds", token)
	}
	return &parser.ScopeSelector{Kind: kind, StartLine: start, EndLine: end, Exact: exact}, nil
}

func positiveSelectorLine(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	line, err := strconv.Atoi(text)
	return line, err == nil && line > 0
}

func legacyAtLocation(path string) bool {
	colon := strings.LastIndexByte(path, ':')
	if colon < 1 || colon == len(path)-1 {
		return false
	}
	start, _, _ := strings.Cut(path[colon+1:], "-")
	// The old PATH:LINE parser owns numeric-looking suffixes, including invalid
	// or out-of-range ones. Never reinterpret them as selector paths.
	return start != "" && (start[0] >= '0' && start[0] <= '9' || start[0] == '+' || start[0] == '-')
}
