package initcommand

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/fantasy"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

const initToolOutputLimit = 32 << 10

type initReadInput struct {
	Path      string `json:"path" description:"Path relative to the directory being documented"`
	StartLine int    `json:"start_line,omitempty" description:"Inclusive 1-based start line; defaults to 1"`
	EndLine   int    `json:"end_line,omitempty" description:"Inclusive end line; defaults to at most 400 lines"`
}

type initGrepInput struct {
	Query      string   `json:"query" description:"Literal text to find, or a regular expression when regex is true"`
	Paths      []string `json:"paths,omitempty" description:"Paths relative to the directory being documented; defaults to the directory"`
	Regex      bool     `json:"regex,omitempty"`
	IgnoreCase bool     `json:"ignore_case,omitempty"`
	Limit      int      `json:"limit,omitempty" description:"Maximum matching lines; defaults to 40 and cannot exceed 100"`
}

type initTreeInput struct {
	Depth int `json:"depth,omitempty" description:"Maximum depth; defaults to 2 and cannot exceed 4"`
}

func initSystemPrompt(root, directory string) string {
	return fmt.Sprintf(`You document one source directory for Grepple. The repository root is %q and the current directory is %q.
Use directory_tree first, then grep_code and read_file as needed to understand every listed file. Tools are read-only and confined to the current directory. Base descriptions, source kinds and area proposals on source evidence, not file names alone. Independently identify cohesive capabilities or workflows and propose reusable lowercase-hyphenated areas for the source and test files that own them, including when there are no existing area tags. Leave areas empty only if the inspected source gives no meaningful feature evidence. Never tag all files merely because they share a directory. Every file kind must be exactly one of production, test, fixture, generated, vendor, or unknown; use unknown only when evidence is insufficient. Your final response must contain only valid grepple.yaml YAML with description, responsibilities, and one entry for every authoritative file path and checksum. Preserve existing areas; for every new area on a file, include a matching area_proposals entry with action: add and concrete evidence naming a selected source file and positive line number (e.g. example.go:12) explaining ownership. Do not write SOURCE:LINE or SOURCE:12 literally. For removal suggestions use action: remove with evidence; do not remove hand-authored tags.`, root, directory)
}

func initTools(application cliruntime.Context, root, directory string) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		fantasy.NewAgentTool("directory_tree", "List the Grepple-selected files and subdirectories under the current directory.", func(ctx context.Context, input initTreeInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			value, err := runInitTree(ctx, application, root, directory, input)
			return initToolResponse(value, err), nil
		}),
		fantasy.NewAgentTool("grep_code", "Search the current directory with Grepple and return matching source lines with file and line numbers.", func(ctx context.Context, input initGrepInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			value, err := runInitGrep(ctx, application, root, directory, input)
			return initToolResponse(value, err), nil
		}),
		fantasy.NewAgentTool("read_file", "Read a bounded file range with 1-based line numbers.", func(ctx context.Context, input initReadInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			value, err := runInitRead(ctx, directory, input)
			return initToolResponse(value, err), nil
		}),
	}
}

func runInitRead(ctx context.Context, directory string, input initReadInput) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := confinedToolPath(directory, input.Path)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := search.SplitLines(string(content))
	start := input.StartLine
	if start == 0 {
		start = 1
	}
	end := input.EndLine
	if end == 0 || end > start+399 {
		end = start + 399
	}
	if start < 1 || end < start || start > len(lines) {
		return "", fmt.Errorf("invalid line range %d-%d for %s (%d lines)", start, end, input.Path, len(lines))
	}
	end = min(end, len(lines))
	var output strings.Builder
	fmt.Fprintf(&output, "%s:%d-%d\n", filepath.ToSlash(input.Path), start, end)
	for index := start; index <= end; index++ {
		fmt.Fprintf(&output, "%d│%s\n", index, lines[index-1])
	}
	return boundedInitToolOutput(output.String()), nil
}

func runInitGrep(ctx context.Context, application cliruntime.Context, root, directory string, input initGrepInput) (string, error) {
	if strings.TrimSpace(input.Query) == "" {
		return "", fmt.Errorf("query is required")
	}
	limit := input.Limit
	if limit == 0 {
		limit = 40
	}
	if limit < 1 || limit > 100 {
		return "", fmt.Errorf("limit must be between 1 and 100")
	}
	allowed := []string{directory}
	if len(input.Paths) != 0 {
		allowed = make([]string, 0, len(input.Paths))
		for _, value := range input.Paths {
			path, err := confinedToolPath(directory, value)
			if err != nil {
				return "", err
			}
			allowed = append(allowed, path)
		}
	}
	candidates, err := initCandidatePaths(ctx, application, root, directory)
	if err != nil {
		return "", err
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		for _, path := range allowed {
			if pathWithinDirectory(root, path, candidate) {
				filtered = append(filtered, candidate)
				break
			}
		}
	}
	params := search.Params{Query: input.Query, Regex: input.Regex, IgnoreCase: input.IgnoreCase, Root: root, SkipSegments: true}
	if err := search.ConfigureSourcePolicy(&params, application.Repository()); err != nil {
		return "", err
	}
	matches, err := search.Files(params, filtered)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	count := 0
	for _, match := range matches {
		lines := search.SplitLines(match.Content)
		for line := 1; line <= len(lines); line++ {
			if !match.MatchLines[line] {
				continue
			}
			fmt.Fprintf(&output, "%s:%d│%s\n", displayPath(root, match.File), line, lines[line-1])
			count++
			if count == limit {
				return boundedInitToolOutput(output.String()), nil
			}
		}
	}
	if count == 0 {
		return "No matches.", nil
	}
	return boundedInitToolOutput(output.String()), nil
}

func runInitTree(ctx context.Context, application cliruntime.Context, root, directory string, input initTreeInput) (string, error) {
	depth := input.Depth
	if depth == 0 {
		depth = 2
	}
	if depth < 1 || depth > 4 {
		return "", fmt.Errorf("depth must be between 1 and 4")
	}
	paths, err := initCandidatePaths(ctx, application, root, directory)
	if err != nil {
		return "", err
	}
	entries := map[string]bool{}
	for _, path := range paths {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(root, filepath.FromSlash(path))
		}
		relative, relErr := filepath.Rel(directory, absolute)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) > depth {
			parts = parts[:depth]
		}
		for index := range parts {
			entry := strings.Join(parts[:index+1], "/")
			entries[entry] = index < len(parts)-1 || len(strings.Split(filepath.ToSlash(relative), "/")) > depth
		}
	}
	ordered := make([]string, 0, len(entries))
	for entry := range entries {
		ordered = append(ordered, entry)
	}
	sort.Strings(ordered)
	var output strings.Builder
	output.WriteString(".\n")
	for _, entry := range ordered {
		indent := strings.Repeat("  ", strings.Count(entry, "/"))
		name := filepath.Base(filepath.FromSlash(entry))
		if entries[entry] {
			name += "/"
		}
		fmt.Fprintf(&output, "%s- %s\n", indent, name)
	}
	return boundedInitToolOutput(output.String()), nil
}

func initCandidatePaths(ctx context.Context, application cliruntime.Context, root, directory string) ([]string, error) {
	params := search.Params{Files: true, Root: root}
	if err := search.ConfigureSourcePolicy(&params, application.Repository()); err != nil {
		return nil, err
	}
	paths, err := search.ListFilePathsContext(ctx, params, nil)
	if err != nil {
		return nil, err
	}
	return pathsWithinDirectory(root, directory, paths), nil
}

func pathsWithinDirectory(root, directory string, paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if pathWithinDirectory(root, directory, path) {
			absolute := path
			if !filepath.IsAbs(absolute) {
				absolute = filepath.Join(root, filepath.FromSlash(path))
			}
			result = append(result, filepath.Clean(absolute))
		}
	}
	return result
}

func pathWithinDirectory(root, directory, path string) bool {
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, filepath.FromSlash(path))
	}
	relative, err := filepath.Rel(directory, filepath.Clean(absolute))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func confinedToolPath(directory, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("path is required")
	}
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, filepath.FromSlash(value))
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolvedDirectory, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the current directory", value)
	}
	return path, nil
}

func initToolResponse(value string, err error) fantasy.ToolResponse {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return fantasy.NewTextResponse(value)
}

func boundedInitToolOutput(value string) string {
	if len(value) <= initToolOutputLimit {
		return value
	}
	return value[:initToolOutputLimit] + "\n… output truncated; narrow the request …"
}
