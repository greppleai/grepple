package extract

import (
	"bytes"
	"go/format"
	goparser "go/parser"
	"go/scanner"
	"go/token"
	"strings"
)

// normalizeGoUnderlyingType produces a compact, syntactically lossless spelling.
// In particular, it retains separators between names in function parameters and
// struct fields while removing formatting-only whitespace.
func normalizeGoUnderlyingType(expression string) (string, bool) {
	parsed, err := goparser.ParseExpr(expression)
	if err != nil {
		return "", false
	}
	var formatted bytes.Buffer
	if err := format.Node(&formatted, token.NewFileSet(), parsed); err != nil {
		return "", false
	}

	fileSet := token.NewFileSet()
	file := fileSet.AddFile("underlying.go", fileSet.Base(), formatted.Len())
	var lexer scanner.Scanner
	lexer.Init(file, formatted.Bytes(), nil, 0)
	var result strings.Builder
	previous := token.ILLEGAL
	for {
		_, current, literal := lexer.Scan()
		if current == token.EOF {
			break
		}
		if current == token.SEMICOLON && literal == "\n" {
			literal = ";"
		}
		if literal == "" {
			literal = current.String()
		}
		if goWordToken(previous) && goWordToken(current) {
			result.WriteByte(' ')
		}
		result.WriteString(literal)
		previous = current
	}
	return strings.TrimSuffix(result.String(), ";"), true
}

// normalizeGoType canonicalizes a Go schema type without removing whitespace that
// separates tokens. tuple is a synthetic schema type, so its Go components are
// normalized independently.
func normalizeGoType(value string) string {
	value = decodeMermaidGeneric(value)
	if strings.HasPrefix(value, "tuple<") && strings.HasSuffix(value, ">") {
		components := splitParameters(value[len("tuple<") : len(value)-1])
		for index := range components {
			components[index] = normalizeGoType(components[index])
		}
		return "tuple<" + strings.Join(components, ",") + ">"
	}
	if strings.HasPrefix(value, "...") {
		return "..." + normalizeGoType(strings.TrimSpace(strings.TrimPrefix(value, "...")))
	}
	if normalized, ok := normalizeGoUnderlyingType(value); ok {
		return normalized
	}
	// Preserve the historical best effort for placeholders and malformed types;
	// valid Go expressions always take the token-aware path above.
	return strings.Join(strings.Fields(value), "")
}

func goWordToken(value token.Token) bool {
	return value == token.IDENT || value.IsKeyword()
}
