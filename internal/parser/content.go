package parser

import (
	"path/filepath"
	"strings"
)

// LanguageFor classifies a path by extension. Unsupported source formats are
// classified as text and use the plain-text segment fallback.
func LanguageFor(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	for _, capability := range languageCapabilities {
		for _, candidate := range capability.Extensions {
			if extension == candidate {
				return capability.ID
			}
		}
	}
	switch extension {
	case ".md", ".markdown", ".mdown", ".mkd":
		return "markdown"
	default:
		return "text"
	}
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
