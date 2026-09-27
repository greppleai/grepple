// Package directorymeta owns grepple.yaml directory descriptions.
package directorymeta

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/filedigest"
	"go.yaml.in/yaml/v3"
)

const FileName = "grepple.yaml"

type File struct {
	Path        string   `yaml:"path" json:"path"`
	Description string   `yaml:"description" json:"description"`
	Kind        string   `yaml:"kind" json:"kind"`
	Checksum    string   `yaml:"checksum" json:"checksum"`
	Areas       []string `yaml:"areas,omitempty" json:"areas,omitempty"`
}

type AreaProposal struct {
	Path     string `yaml:"path" json:"path"`
	Area     string `yaml:"area" json:"area"`
	Action   string `yaml:"action" json:"action"`
	Evidence string `yaml:"evidence" json:"evidence"`
}

type Metadata struct {
	Description      string         `yaml:"description" json:"description"`
	Responsibilities []string       `yaml:"responsibilities" json:"responsibilities"`
	Files            []File         `yaml:"files" json:"files"`
	AreaProposals    []AreaProposal `yaml:"-" json:"-"` // Review-only; never persisted in grepple.yaml.
}

func Read(directory string) (Metadata, error) {
	path := filepath.Join(directory, FileName)
	content, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return metadata, nil
}

func Write(directory string, metadata Metadata) error {
	content, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, FileName), content, 0o644)
}

// Directories returns the root and every ancestor directory containing selected files.
func Directories(root string, files []string) ([]string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{absoluteRoot: true}
	for _, file := range files {
		absolute := file
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(absoluteRoot, filepath.FromSlash(file))
		}
		directory := filepath.Dir(filepath.Clean(absolute))
		for {
			relative, relErr := filepath.Rel(absoluteRoot, directory)
			if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				break
			}
			set[directory] = true
			if directory == absoluteRoot {
				break
			}
			directory = filepath.Dir(directory)
		}
	}
	result := make([]string, 0, len(set))
	for directory := range set {
		result = append(result, directory)
	}
	sort.Slice(result, func(i, j int) bool {
		di, dj := strings.Count(filepath.ToSlash(result[i]), "/"), strings.Count(filepath.ToSlash(result[j]), "/")
		if di != dj {
			return di < dj
		}
		return result[i] < result[j]
	})
	return result, nil
}

func FilesForDirectory(root, directory string, paths []string) ([]File, error) {
	result := []File{}
	for _, path := range paths {
		if filepath.Base(path) == FileName {
			continue
		}
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(path))
		}
		if filepath.Clean(filepath.Dir(absolute)) != filepath.Clean(directory) {
			continue
		}
		digest, err := filedigest.SHA256Hex(absolute)
		if err != nil {
			return nil, err
		}
		result = append(result, File{Path: filepath.Base(path), Checksum: digest})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
