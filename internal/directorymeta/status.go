package directorymeta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/filedigest"
)

const (
	StatusCurrent = "current"
	StatusStale   = "stale"
	StatusMissing = "missing"
	StatusInvalid = "invalid"
)

// FileState describes one selected file's directory-metadata coverage.
type FileState struct {
	Description string
	Kind        string
	Areas       []string
	Status      string
	Issues      []string
}

// Inspection describes the freshness of one directory's metadata and selected files.
type Inspection struct {
	Metadata Metadata
	Status   string
	Issues   []string
	Files    map[string]FileState
}

// Inspect compares grepple.yaml with the currently selected direct files in directory.
func Inspect(root, directory string, paths []string) Inspection {
	selected := selectedDirectFiles(root, directory, paths)
	inspection := Inspection{Status: StatusCurrent, Files: make(map[string]FileState, len(selected))}
	metadata, err := Read(directory)
	if err != nil {
		status := StatusInvalid
		if errors.Is(err, os.ErrNotExist) {
			status = StatusMissing
		}
		inspection.Status = status
		issue := "grepple.yaml is missing"
		if status == StatusInvalid {
			issue = "grepple.yaml is invalid: " + err.Error()
		}
		inspection.Issues = []string{issue}
		for _, name := range selected {
			inspection.Files[name] = FileState{Status: status, Issues: []string{"directory metadata is " + status}}
		}
		return inspection
	}
	inspection.Metadata = metadata
	if strings.TrimSpace(metadata.Description) == "" {
		inspection.addIssue(StatusInvalid, "directory description is empty")
	}
	if len(metadata.Responsibilities) == 0 {
		inspection.addIssue(StatusInvalid, "directory responsibilities are empty")
	}
	entries := make(map[string]File, len(metadata.Files))
	for _, file := range metadata.Files {
		name := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if name == "." || name == "" || strings.Contains(name, "/") {
			inspection.addIssue(StatusInvalid, fmt.Sprintf("invalid metadata file path %q", file.Path))
			continue
		}
		if _, exists := entries[name]; exists {
			inspection.addIssue(StatusInvalid, fmt.Sprintf("duplicate metadata file path %q", file.Path))
			continue
		}
		entries[name] = file
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedSet[name] = true
		entry, exists := entries[name]
		if !exists {
			inspection.Files[name] = FileState{Status: StatusMissing, Issues: []string{"file entry is missing from grepple.yaml"}}
			inspection.addIssue(StatusStale, fmt.Sprintf("missing file entry %q", name))
			continue
		}
		kind := normalizedSourceKind(entry.Kind)
		state := FileState{Description: strings.TrimSpace(entry.Description), Kind: kind, Areas: entry.Areas, Status: StatusCurrent}
		if state.Description == "" {
			state.Status = StatusInvalid
			state.Issues = append(state.Issues, "file description is empty")
			inspection.addIssue(StatusInvalid, fmt.Sprintf("empty description for %q", name))
		}
		for _, issue := range ValidateAreas(entry.Areas) {
			state.Status = StatusInvalid
			state.Issues = append(state.Issues, issue)
			inspection.addIssue(StatusInvalid, fmt.Sprintf("%s for %q", issue, name))
		}
		if kind == "invalid" {
			state.Status = StatusInvalid
			state.Issues = append(state.Issues, "source kind is invalid")
			inspection.addIssue(StatusInvalid, fmt.Sprintf("invalid source kind %q for %q", entry.Kind, name))
		}
		checksum, checksumErr := checksumFile(filepath.Join(directory, filepath.FromSlash(name)))
		if checksumErr != nil {
			state.Status = StatusInvalid
			state.Issues = append(state.Issues, checksumErr.Error())
			inspection.addIssue(StatusInvalid, fmt.Sprintf("read %q: %v", name, checksumErr))
		} else if strings.TrimSpace(entry.Checksum) == "" || !strings.EqualFold(entry.Checksum, checksum) {
			if state.Status != StatusInvalid {
				state.Status = StatusStale
			}
			state.Issues = append(state.Issues, "checksum does not match current content")
			inspection.addIssue(StatusStale, fmt.Sprintf("stale checksum for %q", name))
		}
		inspection.Files[name] = state
	}
	for name := range entries {
		if selectedSet[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(name))); errors.Is(err, os.ErrNotExist) {
			inspection.addIssue(StatusStale, fmt.Sprintf("metadata references missing file %q", name))
		} else if err != nil {
			inspection.addIssue(StatusInvalid, fmt.Sprintf("inspect metadata file %q: %v", name, err))
		}
	}
	sort.Strings(inspection.Issues)
	return inspection
}

func (inspection *Inspection) addIssue(status, issue string) {
	if inspection.Status != StatusInvalid && (status == StatusInvalid || inspection.Status == StatusCurrent) {
		inspection.Status = status
	}
	inspection.Issues = append(inspection.Issues, issue)
}

func selectedDirectFiles(root, directory string, paths []string) []string {
	set := map[string]bool{}
	for _, path := range paths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(path))
		}
		if filepath.Clean(filepath.Dir(absolute)) != filepath.Clean(directory) || filepath.Base(absolute) == FileName {
			continue
		}
		set[filepath.Base(absolute)] = true
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func checksumFile(path string) (string, error) { return filedigest.SHA256Hex(path) }

func normalizedSourceKind(value string) string {
	kind := strings.ToLower(strings.TrimSpace(value))
	if kind == "" {
		return "unknown"
	}
	switch kind {
	case "production", "test", "fixture", "generated", "vendor", "unknown":
		return kind
	default:
		return "invalid"
	}
}
