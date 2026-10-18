package directorymeta

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileStateV1 binds one selected source file to its validated annotation.
// Non-current metadata is a review hint, never an authoritative search tag.
type FileStateV1 struct {
	File   FileV1
	Status string
	Issues []string
}

type InspectionV1 struct {
	Status string
	Issues []string
	Files  map[string]FileStateV1
}

// InspectDirectoryV1 checks only selected direct files in one directory.
// Source scope is supplied by the caller; persisted entries cannot expand it.
func InspectDirectoryV1(root, directory string, paths []string) InspectionV1 {
	document, err := ReadDocumentV1(root)
	return inspectDirectorySnapshotV1(root, directory, paths, document, err)
}

// inspectDirectorySnapshotV1 reuses a single parsed snapshot across a selected
// source inventory; it never promotes annotations from unselected files.
func inspectDirectorySnapshotV1(root, directory string, paths []string, document DocumentV1, err error) InspectionV1 {
	selected := selectedDirectFiles(root, directory, paths)
	inspection := InspectionV1{Status: StatusCurrent, Files: make(map[string]FileStateV1, len(selected))}
	if err != nil {
		status := StatusInvalid
		if errors.Is(err, os.ErrNotExist) {
			status = StatusMissing
		}
		inspection.Status = status
		inspection.Issues = append(inspection.Issues, fmt.Sprintf("metadata snapshot is %s: %v", status, err))
		for _, name := range selected {
			inspection.Files[name] = FileStateV1{Status: status, Issues: []string{"metadata snapshot is " + status}}
		}
		return inspection
	}
	key, err := DirectoryKey(root, directory)
	if err != nil {
		inspection.Status = StatusInvalid
		inspection.Issues = append(inspection.Issues, err.Error())
		return inspection
	}
	entry, exists := document.Directory[key]
	if !exists {
		inspection.Status = StatusMissing
		inspection.Issues = append(inspection.Issues, "directory entry is missing from .grepple/grepple.yaml")
		for _, name := range selected {
			inspection.Files[name] = FileStateV1{Status: StatusMissing, Issues: []string{"directory entry is missing"}}
		}
		return inspection
	}
	entries := make(map[string]FileV1, len(entry.Files))
	for _, file := range entry.Files {
		entries[file.Name] = file
	}
	selectedNames := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedNames[name] = true
		file, exists := entries[name]
		if !exists {
			inspection.Files[name] = FileStateV1{Status: StatusMissing, Issues: []string{"file entry is missing from .grepple/grepple.yaml"}}
			inspection.addIssue(StatusStale, fmt.Sprintf("missing file entry %q", name))
			continue
		}
		state := FileStateV1{File: file, Status: StatusCurrent}
		digest, digestErr := checksumFile(filepath.Join(directory, name))
		if digestErr != nil {
			state.Status = StatusInvalid
			state.Issues = append(state.Issues, digestErr.Error())
			inspection.addIssue(StatusInvalid, fmt.Sprintf("read %q: %v", name, digestErr))
		} else if file.Checksum != digest {
			state.Status = StatusStale
			state.Issues = append(state.Issues, "checksum does not match current content")
			inspection.addIssue(StatusStale, fmt.Sprintf("stale checksum for %q", name))
		}
		inspection.Files[name] = state
	}
	for name := range entries {
		if selectedNames[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(directory, name)); errors.Is(err, os.ErrNotExist) {
			inspection.addIssue(StatusStale, fmt.Sprintf("metadata references missing file %q", name))
		} else if err != nil {
			inspection.addIssue(StatusInvalid, fmt.Sprintf("inspect %q: %v", name, err))
		}
	}
	sort.Strings(inspection.Issues)
	return inspection
}

func (inspection *InspectionV1) addIssue(status, issue string) {
	if inspection.Status != StatusInvalid && (status == StatusInvalid || inspection.Status == StatusCurrent) {
		inspection.Status = status
	}
	inspection.Issues = append(inspection.Issues, strings.TrimSpace(issue))
}
