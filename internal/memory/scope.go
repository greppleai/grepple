package memory

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	scopeLimit       = 32
	scopeBytesLimit  = 256
	scopePrefixLimit = 32
)

type scopeKind uint8

const (
	directoryScope scopeKind = iota + 1
	directScope
	recursiveScope
)

type scopePattern struct {
	value, prefix, basename string
	kind                    scopeKind
}

// NormalizeScope validates one stored canonical scope. It deliberately does not
// inspect the filesystem: an existing memory remains readable after its prefix
// moves or is deleted.
func NormalizeScope(value string) (string, error) {
	pattern, err := parseScope(value)
	if err != nil {
		return "", err
	}
	return pattern.value, nil
}

// ScopeForStore validates command input and requires its literal prefix to be a
// non-symlinked directory beneath root. A bare directory input is canonicalized
// to a trailing-slash directory scope; stored headers never contain bare scopes.
func ScopeForStore(root, value string) (string, error) {
	if !strings.Contains(value, "*") && !strings.HasSuffix(value, "/") {
		if err := validateScopePrefix(value); err != nil {
			return "", err
		}
		value += "/"
	}
	pattern, err := parseScope(value)
	if err != nil {
		return "", err
	}
	if err := validateScopeDirectory(root, pattern.prefix); err != nil {
		return "", err
	}
	return pattern.value, nil
}

// ScopeMatches compares a canonical scope only with an already selected,
// root-relative result path. It never expands a glob or reads the filesystem.
func ScopeMatches(scope, candidate string) bool {
	pattern, err := parseScope(scope)
	if err != nil || !ValidPath(candidate) || !strings.HasPrefix(candidate, pattern.prefix+"/") {
		return false
	}
	relative := strings.TrimPrefix(candidate, pattern.prefix+"/")
	if relative == "" {
		return false
	}
	switch pattern.kind {
	case directoryScope:
		return true
	case directScope:
		return !strings.Contains(relative, "/") && basenameMatches(pattern.basename, relative)
	case recursiveScope:
		return basenameMatches(pattern.basename, path.Base(relative))
	default:
		return false
	}
}

func parseScope(value string) (scopePattern, error) {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > scopeBytesLimit || strings.ContainsAny(value, "\\:\x00") {
		return scopePattern{}, errors.New("invalid memory scope")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return scopePattern{}, errors.New("invalid memory scope")
		}
	}
	if strings.HasSuffix(value, "/") {
		prefix := strings.TrimSuffix(value, "/")
		if err := validateScopePrefix(prefix); err != nil {
			return scopePattern{}, err
		}
		return scopePattern{value: prefix + "/", prefix: prefix, kind: directoryScope}, nil
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return scopePattern{}, errors.New("memory scope must name a directory or supported glob")
	}
	basename := parts[len(parts)-1]
	if err := validateScopeBasename(basename); err != nil {
		return scopePattern{}, err
	}
	kind := directScope
	prefixParts := parts[:len(parts)-1]
	if len(prefixParts) > 0 && prefixParts[len(prefixParts)-1] == "**" {
		kind = recursiveScope
		prefixParts = prefixParts[:len(prefixParts)-1]
	}
	prefix := strings.Join(prefixParts, "/")
	if err := validateScopePrefix(prefix); err != nil {
		return scopePattern{}, err
	}
	suffix := basename
	if kind == recursiveScope {
		suffix = "**/" + basename
	}
	return scopePattern{value: prefix + "/" + suffix, prefix: prefix, basename: basename, kind: kind}, nil
}

func validateScopePrefix(value string) error {
	if !ValidPath(value) || strings.HasSuffix(value, "/") {
		return errors.New("invalid memory scope prefix")
	}
	parts := strings.Split(value, "/")
	if len(parts) > scopePrefixLimit {
		return errors.New("memory scope prefix is too deep")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || part == ".git" || part == ".grepple" || strings.ContainsAny(part, "*?[]{}\\:\x00") {
			return errors.New("invalid memory scope prefix")
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return errors.New("invalid memory scope prefix")
			}
		}
	}
	return nil
}

func validateScopeBasename(value string) error {
	if value == "" || !utf8.ValidString(value) || !strings.Contains(value, "*") || strings.Contains(value, "**") || strings.ContainsAny(value, "/?[]{}\\:\x00") {
		return errors.New("invalid memory scope basename glob")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return errors.New("invalid memory scope basename glob")
		}
	}
	return nil
}

func validateScopeDirectory(root, prefix string) error {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	parts := strings.Split(prefix, "/")
	for i := range parts {
		name := strings.Join(parts[:i+1], "/")
		info, err := dir.Lstat(name)
		if err != nil {
			return fmt.Errorf("memory scope prefix %q: %w", prefix, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("memory scope prefix %q is not a safe directory", prefix)
		}
	}
	return nil
}

func basenameMatches(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = strings.TrimPrefix(value, parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		at := strings.Index(value, part)
		if at < 0 {
			return false
		}
		value = value[at+len(part):]
	}
	return strings.HasSuffix(value, parts[len(parts)-1])
}
