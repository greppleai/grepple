package boundaries

import (
	"testing"

	"github.com/greppleai/grepple/search"
)

func TestBuildCachedGraphUsesInjectedNavigationWithoutCache(t *testing.T) {
	built := false
	output, state, err := BuildCachedGraph([]string{"."}, 7, false, Dependencies{
		ResolvePaths: func([]string) ([]string, error) { return []string{"main.go"}, nil },
		BuildGraph: func(paths []string, maxFiles int, options search.NavigationBuildOptions) GraphOutput {
			built = len(paths) == 1 && paths[0] == "main.go" && maxFiles == 7 && options.DisableCache
			return GraphOutput{Files: 1}
		},
	})
	if err != nil || !built || state != "disabled" || output.Files != 1 {
		t.Fatalf("output=%+v state=%q built=%t err=%v", output, state, built, err)
	}
}

func TestRunRejectsInvalidBoundaryLimit(t *testing.T) {
	if err := newWithDependencies(Dependencies{}).Run([]string{"--limit", "-1"}); err == nil {
		t.Fatal("negative limit accepted")
	}
}
