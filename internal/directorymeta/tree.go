package directorymeta

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// TreeSummary renders a bounded directory-only tree for caller-selected files.
func TreeSummary(root string, files []string, depth int) string {
	if depth < 1 {
		return ""
	}
	directories, err := Directories(root, files)
	if err != nil {
		return ""
	}
	allowed := make(map[string]bool, len(directories))
	for _, directory := range directories {
		allowed[filepath.Clean(directory)] = true
	}
	var output strings.Builder
	rootInspection := Inspect(root, root, files)
	output.WriteString(".")
	if rootInspection.Metadata.Description != "" {
		output.WriteString(" — " + rootInspection.Metadata.Description)
	}
	output.WriteString(metadataStatusSuffix(rootInspection.Status))
	output.WriteByte('\n')
	appendDirectorySummary(&output, root, root, "", depth, allowed, files)
	return strings.TrimSpace(output.String())
}

func appendDirectorySummary(output *strings.Builder, root, directory, prefix string, depth int, allowed map[string]bool, files []string) {
	if depth == 0 {
		return
	}
	children := make([]string, 0)
	for candidate := range allowed {
		if candidate != filepath.Clean(directory) && filepath.Dir(candidate) == filepath.Clean(directory) {
			children = append(children, candidate)
		}
	}
	sort.Strings(children)
	for index, child := range children {
		last := index == len(children)-1
		branch, next := "├── ", prefix+"│   "
		if last {
			branch, next = "└── ", prefix+"    "
		}
		inspection := Inspect(root, child, files)
		fmt.Fprintf(output, "%s%s%s/", prefix, branch, filepath.Base(child))
		if inspection.Metadata.Description != "" {
			output.WriteString(" — " + inspection.Metadata.Description)
		}
		output.WriteString(metadataStatusSuffix(inspection.Status))
		output.WriteByte('\n')
		appendDirectorySummary(output, root, child, next, depth-1, allowed, files)
	}
}

func metadataStatusSuffix(status string) string {
	switch status {
	case StatusMissing, StatusStale, StatusInvalid:
		return " [metadata: " + status + "]"
	default:
		return ""
	}
}
