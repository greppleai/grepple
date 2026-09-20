package tree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

// Request contains parsed tree command options.
type Request struct {
	cliruntime.CommonArgs
	Depth int    `arg:"--depth" default:"2" placeholder:"N" help:"levels to descend"`
	JSON  bool   `arg:"--json" help:"print raw JSON"`
	Repo  string `arg:"positional,required" placeholder:"OWNER/REPOSITORY"`
	Path  string `arg:"positional" placeholder:"PATH"`
}

// Description returns the command description used by the argument parser.
func (Request) Description() string {
	return "Show the directory tree of an indexed repository."
}

type treeNode struct {
	name     string
	dir      bool
	children map[string]*treeNode
}

// Run executes the tree command.
func Run(args []string, dependencies Dependencies) error {
	values := Request{Depth: 2}
	parser, err := arg.NewParser(arg.Config{Program: "grepple tree"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(dependencies.stdout())
			return nil
		}
		return err
	}
	if values.Depth < 1 {
		return fmt.Errorf("--depth must be a positive integer")
	}
	base := dependencies.serverDefault(values.Server)
	data, err := FetchContext(context.Background(), base, values, dependencies)
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(data)
	}
	return printTree(data, dependencies)
}

// FetchContext retrieves a directory listing from the server.
func FetchContext(ctx context.Context, base string, values Request, dependencies Dependencies) (api.TreeResponse, error) {
	target, err := url.Parse(strings.TrimRight(base, "/") + "/public/tree")
	if err != nil {
		return api.TreeResponse{}, err
	}
	query := target.Query()
	query.Set("repo", values.Repo)
	if values.Path != "" {
		query.Set("path", values.Path)
	}
	query.Set("depth", fmt.Sprint(values.Depth))
	target.RawQuery = query.Encode()
	req, err := dependencies.newRequest(http.MethodGet, target.String(), "", nil)
	if err != nil {
		return api.TreeResponse{}, err
	}
	req = req.WithContext(ctx)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return api.TreeResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return api.TreeResponse{}, fmt.Errorf("server %s returned %d: %s", base, response.StatusCode, string(body))
	}
	var data api.TreeResponse
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return api.TreeResponse{}, err
	}
	return data, nil
}

// printTree renders the listing as a repo[/path] header plus the indented
// tree, exiting 1 when the listing is empty.
func printTree(data api.TreeResponse, dependencies Dependencies) error {
	header := data.Repo
	if data.Path != "" && data.Path != "." {
		header += "/" + data.Path
	}
	if err := cliruntime.NewOutput(dependencies.stdout()).WriteString(header + "\n"); err != nil {
		return err
	}
	if err := renderTree(buildTree(data.Entries), "", dependencies); err != nil {
		return err
	}
	if len(data.Entries) == 0 {
		dependencies.requestExit(1)
	}
	return nil
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
				node.children[part] = child
			}
			node = child
		}
	}
	return root
}

func renderTree(node *treeNode, prefix string, dependencies Dependencies) error {
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
		if err := cliruntime.NewOutput(dependencies.stdout()).WriteString(prefix + branch + child.name + suffix + "\n"); err != nil {
			return err
		}
		if len(child.children) > 0 {
			if err := renderTree(child, next, dependencies); err != nil {
				return err
			}
		}
	}
	return nil
}
