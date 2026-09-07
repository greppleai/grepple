package parser

import (
	"path/filepath"
	"strings"
)

// LanguageFor classifies a path by extension. Unsupported source formats are
// classified as text and use the plain-text segment fallback.
func LanguageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js", ".jsx":
		return "javascript"
	case ".go":
		return "go"
	case ".kt", ".kts":
		return "kotlin"
	case ".java":
		return "java"
	case ".md", ".markdown", ".mdown", ".mkd":
		return "markdown"
	}
	return "text"
}

func splitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
