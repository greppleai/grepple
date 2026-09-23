package render

import (
	"io"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

type treeNode struct {
	name           string
	dir            bool
	description    string
	metadataStatus string
	metadataIssues []string
	children       map[string]*treeNode
}

// Tree renders a tree response as JSON or as a repository header followed by
// a directory-first, indented tree.
func Tree(data api.TreeResponse, destination io.Writer, jsonMode bool) error {
	output := cliruntime.NewOutput(destination)
	if jsonMode {
		return output.WriteJSON(data)
	}
	header := data.Repo
	if data.Path != "" && data.Path != "." {
		header += "/" + data.Path
	}
	if data.Description != "" {
		header += " — " + data.Description
	}
	header += metadataStatusSuffix(data.MetadataStatus)
	if err := output.WriteString(header + "\n"); err != nil {
		return err
	}
	return renderTree(buildTree(data.Entries), "", output)
}

// buildTree folds the flat entry list into a treeNode hierarchy; intermediate
// path components become directory nodes.
func buildTree(entries []api.TreeEntry) *treeNode {
	root := &treeNode{children: map[string]*treeNode{}}
	for _, entry := range entries {
		parts := strings.Split(entry.Path, "/")
		node := root
		for index, part := range parts {
			child := node.children[part]
			if child == nil {
				child = &treeNode{name: part, dir: index < len(parts)-1 || entry.Dir, children: map[string]*treeNode{}}
				if index == len(parts)-1 {
					child.description = entry.Description
					child.metadataStatus = entry.MetadataStatus
					child.metadataIssues = append([]string(nil), entry.MetadataIssues...)
				}
				node.children[part] = child
			}
			node = child
		}
	}
	return root
}

func renderTree(node *treeNode, prefix string, output *cliruntime.Output) error {
	children := make([]*treeNode, 0, len(node.children))
	for _, child := range node.children {
		children = append(children, child)
	}
	sort.Slice(children, func(i, j int) bool {
		if children[i].dir != children[j].dir {
			return children[i].dir
		}
		return children[i].name < children[j].name
	})
	for index, child := range children {
		last := index == len(children)-1
		branch := "├── "
		next := prefix + "│   "
		if last {
			branch = "└── "
			next = prefix + "    "
		}
		suffix := ""
		if child.dir {
			suffix = "/"
		}
		if child.description != "" {
			suffix += " — " + child.description
		}
		suffix += metadataStatusSuffix(child.metadataStatus)
		if err := output.WriteString(prefix + branch + child.name + suffix + "\n"); err != nil {
			return err
		}
		if len(child.children) > 0 {
			if err := renderTree(child, next, output); err != nil {
				return err
			}
		}
	}
	return nil
}

func metadataStatusSuffix(status string) string {
	switch status {
	case "missing", "stale", "invalid":
		return " [metadata: " + status + "]"
	default:
		return ""
	}
}
