package initcommand

import (
	"context"
	"strings"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

// areaInventory always scans the full repository, even for --only-directory.
// It is a prompt hint, not proof of semantic ownership or an extra model call.
func areaInventory(ctx context.Context, application cliruntime.Context, root string) ([]directorymeta.AreaReference, error) {
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return nil, err
	}
	files, err := sourcedomain.ListWithPolicy(ctx, nil, root, policy)
	if err != nil {
		return nil, err
	}
	return directorymeta.AreaIndex(root, files)
}

// areaPrompts reuses earlier generated tags when jobs run sequentially. Parallel
// jobs share the original snapshot, independent of completion order.
func areaPrompts(application cliruntime.Context, root string, concurrency int, inventory []directorymeta.AreaReference) func(context.Context, generationJob) (string, error) {
	first := true
	return func(ctx context.Context, job generationJob) (string, error) {
		if concurrency == 1 {
			if !first {
				var err error
				inventory, err = areaInventory(ctx, application, root)
				if err != nil {
					return "", err
				}
			}
			first = false
		}
		return generationPromptWithAreas(root, job.directory, job.files, inventory)
	}
}

// hasLocalSourceCitation checks only the syntax of an evidence pointer. The
// cited source and semantic ownership still require human review.
func hasLocalSourceCitation(evidence string, files []directorymeta.File) bool {
	for _, file := range files {
		remaining := evidence
		for {
			index := strings.Index(remaining, file.Path+":")
			if index < 0 {
				break
			}
			following := remaining[index+len(file.Path)+1:]
			if len(following) != 0 && following[0] >= '1' && following[0] <= '9' {
				return true
			}
			remaining = following
		}
	}
	return false
}
