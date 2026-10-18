// Package sources owns repository source policy, discovery, metadata-backed classification, and inspection.
package sources

import (
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

// Kind is a source-policy classification derived from a checksum-current V1
// file purpose. It is not a persisted metadata field.
type Kind string

const (
	Unknown    Kind = "unknown"
	Production Kind = "production"
	Test       Kind = "test"
	Fixture    Kind = "fixture"
	Generated  Kind = "generated"
	Vendor     Kind = "vendor"
)

// Kinds returns the classifications accepted by source-policy filters.
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

// Classifier holds one validated metadata snapshot for a selection operation.
// Invalid or old YAML produces no trusted classifications.
type Classifier struct {
	root     string
	document directorymeta.DocumentV1
	kinds    map[string]Kind
}

func NewClassifier(root string) *Classifier {
	if root == "" {
		root = "."
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	root = filepath.Clean(root)
	document, _ := directorymeta.ReadDocumentV1(root)
	return &Classifier{root: root, document: document, kinds: map[string]Kind{}}
}

// Classify is fail-closed: only a file present in a valid V1 directory map
// whose checksum matches current content can confer a source-policy kind.
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
	key, err := directorymeta.DirectoryKey(classifier.root, filepath.Dir(absolute))
	if err == nil {
		for _, entry := range classifier.document.Directory[key].Files {
			if entry.Name != filepath.Base(absolute) {
				continue
			}
			if digest, err := filedigest.SHA256Hex(absolute); err == nil && digest == entry.Checksum {
				kind = kindForPurpose(entry.Purpose)
			}
			break
		}
	}
	classifier.kinds[absolute] = kind
	return kind
}

// Only these explicit purposes map to pre-existing --kind / --production-only
// policy. Documentation and custom purposes are unknown, not production.
func kindForPurpose(value string) Kind {
	switch directorymeta.LabelKey(value) {
	case "implementation", "production":
		return Production
	case "testing", "test":
		return Test
	case "fixture":
		return Fixture
	case "generated":
		return Generated
	case "vendor":
		return Vendor
	default:
		return Unknown
	}
}

// Classify is a convenience for one-off metadata classification.
func Classify(path, root string) Kind { return NewClassifier(root).Classify(path) }

// IsProduction reports whether current metadata classifies path as production.
func IsProduction(path, root string) bool { return Classify(path, root) == Production }
