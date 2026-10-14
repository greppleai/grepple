package analysis

import (
	"path/filepath"
	"strings"
)

// Capture cwd once per selection. Repeated filepath.Abs calls perform Getwd
// filesystem checks, even when every declaration already uses relative paths.
// No global cwd cache: changing directories between commands remains supported.
func graphPathNormalizer() func(string) string {
	cwd, err := filepath.Abs(".")
	return func(value string) string {
		cleaned := filepath.Clean(value)
		if err == nil {
			absolute := cleaned
			if !filepath.IsAbs(absolute) {
				absolute = filepath.Join(cwd, absolute)
			}
			if relative, relativeErr := filepath.Rel(cwd, absolute); relativeErr == nil {
				cleaned = relative
			}
		}
		cleaned = filepath.ToSlash(cleaned)
		cleaned = strings.TrimPrefix(cleaned, "./")
		if cleaned == "" {
			return "."
		}
		return cleaned
	}
}
