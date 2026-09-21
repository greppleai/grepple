package boundaries

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

func testBoundaryDependencies() Dependencies {
	return Dependencies{
		ResolvePaths: func(globs []string) ([]string, error) {
			return search.ListFilePaths(search.Params{Files: true, Globs: globs}, nil)
		},
		BuildGraph: func(paths []string, maxFiles int, options search.NavigationBuildOptions) GraphOutput {
			discovered := len(paths)
			eligible := make([]string, 0, len(paths))
			for _, path := range paths {
				capabilities, ok := parser.CapabilitiesForLanguage(parser.LanguageFor(path))
				if ok && capabilities.Navigation {
					eligible = append(eligible, path)
				}
			}
			var truncation *Truncation
			if maxFiles > 0 && len(eligible) > maxFiles {
				truncation = &Truncation{Reason: "max_files", Limit: maxFiles, Skipped: len(eligible) - maxFiles}
				eligible = eligible[:maxFiles]
			}
			graph, stats := search.BuildNavigationGraphWithOptions(eligible, options)
			return GraphOutput{Files: len(eligible), Sources: SourceSummary{Discovered: discovered, Selected: stats.Attempted, Parsed: stats.Parsed, Skipped: discovered - len(paths) + stats.Skipped, Failed: stats.Failed, Recovered: stats.Recovered}, Declarations: graph.Declarations, Calls: graph.Calls, Fields: graph.Fields, TypeUsages: graph.TypeUsages, MemberAccesses: graph.MemberAccesses, Truncation: truncation}
		},
		CacheDirectory: testBoundaryCacheDirectory,
		Metadata: func(input MetadataInput) *api.ResultMetadata {
			total := len(input.Report.Candidates) + len(input.Report.TypeBoundaries) + len(input.Report.FacadeBypasses)
			return &api.ResultMetadata{Scope: api.ResultScope{Mode: "local", Paths: boundaryDisplayPaths(input.Paths)}, Page: api.ResultPage{Limit: input.Limit, Returned: total, Total: &total, Complete: input.Report.Truncation == nil && input.Report.Sources.Failed == 0 && input.Report.Sources.Recovered == 0}}
		},
	}
}

func testBoundaryCacheDirectory() string {
	if configured := strings.TrimSpace(os.Getenv("GREPPLE_CACHE_DIR")); configured != "" {
		return configured
	}
	working, _ := os.Getwd()
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(working))))[:16]
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "grepple", "cache", digest)
}
func runBoundaries(args []string) error { return Run(args, testBoundaryDependencies()) }
func buildCachedBoundaryGraphForTest(paths []string, maxFiles int, useCache bool) (GraphOutput, string, error) {
	return buildCachedBoundaryGraph(paths, maxFiles, useCache, testBoundaryDependencies())
}
func chdirTemp(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	previous, _ := os.Getwd()
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return directory
}
func writeGraphSource(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	run()
	_ = writer.Close()
	os.Stdout = previous
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
