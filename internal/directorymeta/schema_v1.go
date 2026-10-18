package directorymeta

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// DocumentV1 is the new, file-owned metadata contract. Directory labels are
// derived from current files, never stored in this document.
type DocumentV1 struct {
	Schema    string                 `yaml:"schema" json:"schema"`
	Directory map[string]DirectoryV1 `yaml:"directory" json:"directory"`
}

type DirectoryV1 struct {
	Files []FileV1 `yaml:"files" json:"files"`
}

type FileV1 struct {
	Name            string   `yaml:"name" json:"name"`
	Checksum        string   `yaml:"checksum" json:"checksum"`
	Description     string   `yaml:"description" json:"description"`
	Purpose         string   `yaml:"purpose" json:"purpose"`
	DomainAreas     []string `yaml:"domainAreas,omitempty" json:"domainAreas,omitempty"`
	FunctionalAreas []string `yaml:"functionalAreas,omitempty" json:"functionalAreas,omitempty"`
	Keywords        []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

// ParseDocumentV1 rejects unknown and duplicate YAML keys, including old
// version/directories snapshots, before validating the file manifest.
func ParseDocumentV1(content []byte) (DocumentV1, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var document DocumentV1
	if err := decoder.Decode(&document); err != nil {
		return DocumentV1{}, fmt.Errorf("parse metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return DocumentV1{}, errors.New("multiple YAML documents are not allowed")
		}
		return DocumentV1{}, fmt.Errorf("parse metadata: %w", err)
	}
	if err := document.Normalize(); err != nil {
		return DocumentV1{}, err
	}
	return document, nil
}

// MarshalDocumentV1 normalizes category spelling and order before encoding.
func MarshalDocumentV1(document DocumentV1) ([]byte, error) {
	if err := document.Normalize(); err != nil {
		return nil, err
	}
	return yaml.Marshal(document)
}

// Normalize validates and canonicalizes the document in place. Label values
// are stored in lowercase kebab-case; descriptions and filenames stay intact.
func (document *DocumentV1) Normalize() error {
	if document.Schema != "v1" || document.Directory == nil {
		return errors.New("metadata requires schema: v1 and directory map")
	}
	for key, directory := range document.Directory {
		if key != "." && (key == "" || path.Clean(key) != key || strings.HasPrefix(key, "/") || key == ".." || strings.HasPrefix(key, "../") || strings.ContainsAny(key, "\\:\x00") || !utf8.ValidString(key)) {
			return fmt.Errorf("invalid metadata directory %q", key)
		}
		seen := map[string]bool{}
		for index := range directory.Files {
			file := &directory.Files[index]
			if file.Name == "" || file.Name == "." || file.Name == ".." || strings.ContainsAny(file.Name, "/\\\x00") || !utf8.ValidString(file.Name) {
				return fmt.Errorf("invalid file name %q in %q", file.Name, key)
			}
			if seen[file.Name] {
				return fmt.Errorf("duplicate file name %q in %q", file.Name, key)
			}
			seen[file.Name] = true
			checksum, err := hex.DecodeString(file.Checksum)
			if err != nil || len(checksum) != 32 || file.Checksum != strings.ToLower(file.Checksum) {
				return fmt.Errorf("invalid SHA-256 checksum for %s/%s", key, file.Name)
			}
			file.Description = strings.Join(strings.Fields(file.Description), " ")
			if file.Description == "" || len(file.Description) > 240 {
				return fmt.Errorf("description for %s/%s must be 1-240 bytes", key, file.Name)
			}
			purpose, err := label(file.Purpose)
			if err != nil {
				return fmt.Errorf("purpose for %s/%s: %w", key, file.Name, err)
			}
			file.Purpose = purpose
			if file.DomainAreas, err = labels(file.DomainAreas); err != nil {
				return fmt.Errorf("domainAreas for %s/%s: %w", key, file.Name, err)
			}
			if file.FunctionalAreas, err = labels(file.FunctionalAreas); err != nil {
				return fmt.Errorf("functionalAreas for %s/%s: %w", key, file.Name, err)
			}
			if file.Keywords, err = labels(file.Keywords); err != nil {
				return fmt.Errorf("keywords for %s/%s: %w", key, file.Name, err)
			}
		}
		sort.Slice(directory.Files, func(i, j int) bool { return directory.Files[i].Name < directory.Files[j].Name })
		document.Directory[key] = directory
	}
	return nil
}

// LabelKey makes selector input and stored labels comparable: lowercase with
// all runs of whitespace replaced by one hyphen. Punctuation is preserved.
func LabelKey(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), "-")) }

// ValidLabelV1 accepts a user selector using the same constraints as stored labels.
func ValidLabelV1(value string) bool { _, err := label(value); return err == nil }

func label(value string) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || len(value) > 80 || !utf8.ValidString(value) {
		return "", fmt.Errorf("label must be 1-80 UTF-8 bytes")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("label contains control character")
		}
	}
	return LabelKey(value), nil
}

func labels(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	unique := map[string]string{}
	for _, value := range values {
		normalized, err := label(value)
		if err != nil {
			return nil, err
		}
		key := LabelKey(normalized)
		if _, exists := unique[key]; !exists {
			unique[key] = normalized
		}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, unique[key])
	}
	return result, nil
}
