package sources

import (
	"context"
	"os"
	"sort"
)

// DiscoveryOptions controls repository source discovery.
type DiscoveryOptions struct {
	Root           string
	IgnoreRoot     string
	IgnorePaths    []string
	ProductionOnly bool
}

func (options DiscoveryOptions) ignore() ignoreConfig {
	root := options.Root
	if root == "" {
		root = options.IgnoreRoot
	}
	return ignoreConfig{root: options.IgnoreRoot, patterns: options.IgnorePaths, productionOnly: options.ProductionOnly, classifier: NewClassifier(root)}
}

// Candidates discovers content-search candidates in deterministic path order.
func Candidates(ctx context.Context, globs []string, options DiscoveryOptions) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return collectCandidateFilesConfiguredContext(ctx, globs, options.Root, options.ignore())
}

// Listing discovers listing candidates, treating directory positionals as scope
// roots and other positionals as path filters.
func Listing(ctx context.Context, globs []string, options DiscoveryOptions) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return collectListingFilesConfiguredContext(ctx, globs, options.Root, options.ignore())
}

// ListWithPolicy applies neutral repository policy and returns deterministic
// display paths relative to the process working directory.
func ListWithPolicy(ctx context.Context, globs []string, root string, policy Options) ([]string, error) {
	ReportExplicitBypasses(globs, policy)
	files, err := Listing(ctx, globs, DiscoveryOptions{Root: root, IgnoreRoot: policy.Root(), IgnorePaths: policy.IgnorePaths, ProductionOnly: policy.ProductionOnly})
	if err != nil {
		return nil, err
	}
	cwd, _ := os.Getwd()
	result := make([]string, 0, len(files))
	for _, file := range files {
		result = append(result, displayPathFrom(file, cwd))
	}
	sort.Strings(result)
	return result, nil
}
