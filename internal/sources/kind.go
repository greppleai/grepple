// Package sources owns repository source policy, discovery, metadata-backed classification, and inspection.
package sources

import (
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

// Kind is a language-neutral repository source classification supplied by grepple.yaml.
type Kind string

const (
	Unknown    Kind = "unknown"
	Production Kind = "production"
	Test       Kind = "test"
	Fixture    Kind = "fixture"
	Generated  Kind = "generated"
	Vendor     Kind = "vendor"
)

// Kinds returns the classifications accepted in directory metadata.
func Kinds() []Kind { return []Kind{Production, Test, Fixture, Generated, Vendor, Unknown} }

// ParseKind normalizes one metadata classification. Missing classification is unknown.
func ParseKind(value string) (Kind, bool) {
	kind := Kind(strings.ToLower(strings.TrimSpace(value)))
	if kind == "" {
		return Unknown, true
	}
	for _, candidate := range Kinds() {
		if kind == candidate {
			return kind, true
		}
	}
	return Unknown, false
}

// Classifier caches directory manifests for one source-selection operation.
type Classifier struct {
	root        string
	directories map[string]map[string]directorymeta.File
	kinds       map[string]Kind
}

// NewClassifier constructs an invocation-local metadata classifier.
func NewClassifier(root string) *Classifier {
	if root == "" {
		root = "."
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return &Classifier{root: filepath.Clean(root), directories: map[string]map[string]directorymeta.File{}, kinds: map[string]Kind{}}
}

// Classify returns a fresh metadata classification, or unknown when metadata is
// missing, stale, invalid, or does not classify the file.
func (classifier *Classifier) Classify(path string) Kind {
	if classifier == nil {
		return Unknown
	}
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(classifier.root, filepath.FromSlash(path))
	}
	absolute = filepath.Clean(absolute)
	if kind, ok := classifier.kinds[absolute]; ok {
		return kind
	}
	kind := Unknown
	entry, ok := classifier.entries(filepath.Dir(absolute))[filepath.Base(absolute)]
	if ok {
		if parsed, valid := ParseKind(entry.Kind); valid && parsed != Unknown && strings.TrimSpace(entry.Checksum) != "" {
			if digest, err := filedigest.SHA256Hex(absolute); err == nil && strings.EqualFold(strings.TrimSpace(entry.Checksum), digest) {
				kind = parsed
			}
		}
	}
	classifier.kinds[absolute] = kind
	return kind
}

func (classifier *Classifier) entries(directory string) map[string]directorymeta.File {
	if entries, ok := classifier.directories[directory]; ok {
		return entries
	}
	entries := map[string]directorymeta.File{}
	duplicates := map[string]bool{}
	metadata, err := directorymeta.Read(directory)
	if err == nil {
		for _, file := range metadata.Files {
			name := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
			if name == "" || name == "." || strings.Contains(name, "/") {
				continue
			}
			if duplicates[name] {
				continue
			}
			if _, duplicate := entries[name]; duplicate {
				delete(entries, name)
				duplicates[name] = true
				continue
			}
			entries[name] = file
		}
	}
	classifier.directories[directory] = entries
	return entries
}

// Classify is a convenience for one-off metadata classification.
func Classify(path, root string) Kind { return NewClassifier(root).Classify(path) }

// IsProduction reports whether current metadata classifies path as production.
func IsProduction(path, root string) bool { return Classify(path, root) == Production }
