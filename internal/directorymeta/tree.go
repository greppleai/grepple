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
	inventory, err := BuildInventoryV1(root, files)
	if err != nil {
		return ""
	}
	allowed := make(map[string]bool, len(inventory.Directories))
	for key := range inventory.Directories {
		allowed[key] = true
	}
	var output strings.Builder
	output.WriteString(".")
	appendSummaryLabels(&output, inventory, ".")
	output.WriteString(metadataStatusSuffix(inventory.Directories["."].Status))
	output.WriteByte('\n')
	appendDirectorySummary(&output, ".", "", depth, allowed, inventory)
	return strings.TrimSpace(output.String())
}

func appendDirectorySummary(output *strings.Builder, directory, prefix string, depth int, allowed map[string]bool, inventory InventoryV1) {
	if depth == 0 {
		return
	}
	children := make([]string, 0)
	for candidate := range allowed {
		if candidate != directory && filepath.ToSlash(filepath.Dir(candidate)) == directory {
			children = append(children, candidate)
		}
	}
	sort.Strings(children)
	for index, child := range children {
		branch, next := "├── ", prefix+"│   "
		if index == len(children)-1 {
			branch, next = "└── ", prefix+"    "
		}
		fmt.Fprintf(output, "%s%s%s/", prefix, branch, filepath.Base(child))
		appendSummaryLabels(output, inventory, child)
		output.WriteString(metadataStatusSuffix(inventory.Directories[child].Status))
		output.WriteByte('\n')
		appendDirectorySummary(output, child, next, depth-1, allowed, inventory)
	}
}

func appendSummaryLabels(output *strings.Builder, inventory InventoryV1, directory string) {
	if purposes := inventory.Labels[directory].Purposes; len(purposes) > 0 {
		output.WriteString(" — purposes: " + strings.Join(purposes, ", "))
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
