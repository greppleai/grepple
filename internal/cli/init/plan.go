package initcommand

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type generationJob struct {
	directory string
	files     []directorymeta.File
	skip      bool
}

type generationPlan []generationJob

func (plan generationPlan) needsGeneration() bool {
	for _, job := range plan {
		if !job.skip {
			return true
		}
	}
	return false
}

// planGeneration snapshots the selected files and freshness of each directory
// before any metadata is written, so concurrent workers cannot affect selection.
func planGeneration(ctx context.Context, application cliruntime.Context, globs []string, force bool, onlyDirectory string) (generationPlan, error) {
	root := application.Repository().WorkingDirectory()
	var exactDirectory string
	if onlyDirectory != "" && len(globs) != 0 {
		return nil, fmt.Errorf("--only-directory cannot be combined with positional paths")
	}
	if onlyDirectory != "" {
		resolved, err := confinedDirectory(root, onlyDirectory)
		if err != nil {
			return nil, err
		}
		exactDirectory = resolved
		globs = nil
	}
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return nil, err
	}
	paths, err := sourcedomain.ListWithPolicy(ctx, globs, root, policy)
	if err != nil {
		return nil, err
	}
	if exactDirectory != "" {
		paths = pathsWithinDirectory(root, exactDirectory, paths)
	}
	var directories []string
	if onlyDirectory != "" {
		directories = []string{exactDirectory}
	} else {
		directories, err = directorymeta.Directories(root, paths)
		if err != nil {
			return nil, err
		}
	}
	plan := make(generationPlan, 0, len(directories))
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		job := generationJob{directory: directory}
		if !force && directorymeta.Inspect(root, directory, paths).Status == directorymeta.StatusCurrent {
			job.skip = true
		} else {
			job.files, err = directorymeta.FilesForDirectory(root, directory, paths)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", displayPath(root, directory), err)
			}
		}
		plan = append(plan, job)
	}
	return plan, nil
}

// runGeneration bounds concurrent agent requests and prints results in directory
// order. A failed directory does not prevent independent directories refreshing.
func runGeneration(ctx context.Context, application cliruntime.Context, root string, plan generationPlan, concurrency int, generate func(context.Context, generationJob) (directorymeta.Metadata, error)) error {
	indices := make(chan int, len(plan))
	type result struct {
		index     int
		err       error
		proposals []directorymeta.AreaProposal
	}
	completed := make(chan result, len(plan))
	pending := 0
	for index, job := range plan {
		if !job.skip {
			indices <- index
			pending++
		}
	}
	close(indices)
	workers := concurrency
	if workers > pending {
		workers = pending
	}
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range indices {
				job := plan[index]
				if err := ctx.Err(); err != nil {
					completed <- result{index: index, err: err}
					continue
				}
				metadata, err := generate(ctx, job)
				if err == nil {
					err = directorymeta.Write(job.directory, metadata)
				}
				completed <- result{index: index, err: err, proposals: metadata.AreaProposals}
			}
		}()
	}
	ready := make([]bool, len(plan))
	proposals := make([][]directorymeta.AreaProposal, len(plan))
	results := make([]error, len(plan))
	for index, job := range plan {
		ready[index] = job.skip
	}
	var failures []error
	next := 0
	flush := func() {
		for next < len(plan) && ready[next] {
			job := plan[next]
			path := displayPath(root, job.directory)
			if job.skip {
				fmt.Fprintln(application.Stdout(), "skip", path)
			} else if err := results[next]; err != nil {
				failures = append(failures, fmt.Errorf("generate %s: %w", path, err))
			} else {
				fmt.Fprintln(application.Stdout(), "write", filepath.ToSlash(filepath.Join(path, directorymeta.FileName)))
				for _, proposal := range proposals[next] {
					evidence := strings.Join(strings.Fields(proposal.Evidence), " ")
					fmt.Fprintf(application.Stdout(), "area-proposal %s %s %s/%s: %s\n", proposal.Action, proposal.Area, path, proposal.Path, evidence)
				}
			}
			next++
		}
	}
	flush()
	for range pending {
		result := <-completed
		results[result.index] = result.err
		proposals[result.index] = result.proposals
		ready[result.index] = true
		flush()
	}
	group.Wait()
	return errors.Join(failures...)
}
