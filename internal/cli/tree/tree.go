package tree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

// Request contains parsed tree command options.
type Request struct {
	cliruntime.CommonArgs
	Depth      int      `arg:"--depth" default:"2" placeholder:"N" help:"levels to descend"`
	JSON       bool     `arg:"--json" help:"print raw JSON"`
	Repository string   `arg:"--repo" placeholder:"OWNER/REPOSITORY[@REF]" help:"show one exact indexed repository instead of the local checkout"`
	Repo       string   `arg:"-"`
	Path       string   `arg:"-"`
	Targets    []string `arg:"positional" placeholder:"PATH" help:"local path (default .); legacy remote syntax also accepts OWNER/REPOSITORY [PATH]"`
}

// Description returns the command description used by the argument parser.
func (Request) Description() string {
	return "Show the local source tree by default; --repo selects an exact indexed repository."
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
	remote, err := normalizeRequest(&values)
	if err != nil {
		return err
	}
	var data api.TreeResponse
	if remote {
		base := dependencies.serverDefault(values.Server)
		data, err = FetchContext(context.Background(), base, values, dependencies)
	} else {
		data, err = dependencies.localTree(values.Path, values.Depth)
	}
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(data)
	}
	return printTree(data, dependencies)
}

func normalizeRequest(values *Request) (bool, error) {
	if values == nil {
		return false, fmt.Errorf("tree request is required")
	}
	if values.Repository != "" {
		if len(values.Targets) > 1 {
			return false, fmt.Errorf("tree --repo accepts at most one PATH")
		}
		values.Repo = values.Repository
		if len(values.Targets) == 1 {
			values.Path = values.Targets[0]
		}
		return true, nil
	}
	if len(values.Targets) > 2 {
		return false, fmt.Errorf("tree accepts one local PATH or legacy OWNER/REPOSITORY [PATH]")
	}
	if len(values.Targets) == 2 {
		values.Repo, values.Path = values.Targets[0], values.Targets[1]
		return true, nil
	}
	if len(values.Targets) == 0 {
		values.Path = "."
		return false, nil
	}
	candidate := values.Targets[0]
	if legacyRepositoryTarget(candidate) {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			values.Repo = candidate
			return true, nil
		}
	}
	values.Path = candidate
	return false, nil
}

func legacyRepositoryTarget(value string) bool {
	if strings.HasPrefix(value, ".") || filepath.IsAbs(value) {
		return false
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(value)), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && parts[0] != ".." && parts[1] != ".."
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
