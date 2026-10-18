package initcommand

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"charm.land/fantasy"
	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type generationJobV1 struct {
	directory string
	files     []directorymeta.FileV1
	globs     []string // original source selection; re-evaluated after the model runs
	skip      bool
}
type generationPlanV1 []generationJobV1

func (plan generationPlanV1) needsGeneration() bool {
	for _, job := range plan {
		if !job.skip && len(job.files) != 0 {
			return true
		}
	}
	return false
}

// planGenerationV1 snapshots the caller-selected sources and metadata statuses
// before workers start. Invalid snapshots cannot be silently overwritten.
func planGenerationV1(ctx context.Context, application cliruntime.Context, globs []string, force bool, onlyDirectory string) (generationPlanV1, error) {
	root := application.Repository().WorkingDirectory()
	var exact string
	if onlyDirectory != "" {
		if len(globs) != 0 {
			return nil, fmt.Errorf("--only-directory cannot be combined with positional paths")
		}
		var err error
		exact, err = confinedDirectory(root, onlyDirectory)
		if err != nil {
			return nil, err
		}
	}
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return nil, err
	}
	paths, err := sourcedomain.ListWithPolicy(ctx, globs, root, policy)
	if err != nil {
		return nil, err
	}
	if exact != "" {
		paths = pathsWithinDirectory(root, exact, paths)
	}
	// Do not select a model or make a network request when the shared snapshot
	// is malformed, even under --force. The user must fix or remove it.
	if _, err := directorymeta.ReadDocumentV1(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	inventory, err := directorymeta.BuildInventoryV1(root, paths)
	if err != nil {
		return nil, err
	}
	directories, err := directorymeta.Directories(root, paths)
	if err != nil {
		return nil, err
	}
	if exact != "" {
		directories = []string{exact}
	}
	plan := make(generationPlanV1, 0, len(directories))
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, err := directorymeta.DirectoryKey(root, directory)
		if err != nil {
			return nil, err
		}
		job := generationJobV1{directory: directory, globs: append([]string(nil), globs...)}
		if !force && inventory.Directories[key].Status == directorymeta.StatusCurrent {
			job.skip = true
		} else {
			job.files, err = directorymeta.FilesForDirectoryV1(root, directory, paths)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", displayPath(root, directory), err)
			}
		}
		plan = append(plan, job)
	}
	return plan, nil
}

func generateDirectoryMetadataV1(ctx context.Context, application cliruntime.Context, model fantasy.LanguageModel, root string, job generationJobV1, prompt string) (directorymeta.DirectoryV1, error) {
	return generateDirectoryMetadataWithRunnerV1(ctx, application, model, root, job, prompt, func(ctx context.Context, request agent.Request) (agent.Result, error) {
		return agent.Run(ctx, nil, request)
	})
}

// Keep the agent boundary injectable so retry and validation can be tested
// without provider credentials, model calls or a filesystem-backed fake agent.
func generateDirectoryMetadataWithRunnerV1(ctx context.Context, application cliruntime.Context, model fantasy.LanguageModel, root string, job generationJobV1, prompt string, run func(context.Context, agent.Request) (agent.Result, error)) (directorymeta.DirectoryV1, error) {
	document, err := directorymeta.ReadDocumentV1(root)
	if errors.Is(err, os.ErrNotExist) {
		document = directorymeta.DocumentV1{Schema: "v1", Directory: map[string]directorymeta.DirectoryV1{}}
	} else if err != nil {
		return directorymeta.DirectoryV1{}, err
	}
	key, err := directorymeta.DirectoryKey(root, job.directory)
	if err != nil {
		return directorymeta.DirectoryV1{}, err
	}
	existing := document.Directory[key]
	var validationErr error
	for attempt := 0; attempt < 2; attempt++ {
		result, err := run(ctx, agent.Request{
			Name: "file metadata", Prompt: prompt, SystemPrompt: initSystemPromptV1(root, job.directory),
			ModelFactory: func(context.Context) (fantasy.LanguageModel, error) { return model, nil },
			Tools:        initTools(application, root, job.directory),
		})
		if err != nil {
			return directorymeta.DirectoryV1{}, err
		}
		generated, err := parseGeneratedV1(result.Answer, job.files, existing)
		if err == nil {
			return generated, nil
		}
		validationErr = err
		prompt = fmt.Sprintf("%s\n\nYour previous YAML was invalid: %s. Correct it and return complete YAML only. Previous response:\n%s", prompt, err, result.Answer)
	}
	return directorymeta.DirectoryV1{}, validationErr
}

// Scoped refreshes replace every selected file, but must not discard valid
// human annotations for files excluded by --production-only or ignore rules.
// Removed, symlinked and checksum-stale excluded files are not preserved.
func retainCurrentUnselectedV1(root string, job generationJobV1, entry directorymeta.DirectoryV1) (directorymeta.DirectoryV1, error) {
	document, err := directorymeta.ReadDocumentV1(root)
	if errors.Is(err, os.ErrNotExist) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	key, err := directorymeta.DirectoryKey(root, job.directory)
	if err != nil {
		return entry, err
	}
	selected := make(map[string]bool, len(job.files))
	for _, file := range job.files {
		selected[file.Name] = true
	}
	for _, prior := range document.Directory[key].Files {
		if selected[prior.Name] {
			continue
		}
		path := filepath.Join(job.directory, prior.Name)
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return entry, fmt.Errorf("inspect excluded file %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		checksum, hashErr := filedigest.SHA256Hex(path)
		if hashErr != nil {
			return entry, fmt.Errorf("checksum excluded file %s: %w", path, hashErr)
		}
		if checksum == prior.Checksum {
			entry.Files = append(entry.Files, prior)
		}
	}
	return entry, nil
}

// runGenerationV1 bounds model calls, reselects source scope and checksums
// immediately before writing, and prints results in directory order.
func runGenerationV1(ctx context.Context, application cliruntime.Context, root string, plan generationPlanV1, concurrency int, generate func(context.Context, generationJobV1) (directorymeta.DirectoryV1, error)) error {
	indices := make(chan int, len(plan))
	type result struct {
		index int
		err   error
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
					completed <- result{index, err}
					continue
				}
				entry, err := directorymeta.DirectoryV1{Files: []directorymeta.FileV1{}}, error(nil)
				if len(job.files) != 0 {
					entry, err = generate(ctx, job)
				}
				if err == nil {
					// A model may take minutes. Refuse to write stale checksums if selected
					// source changed during generation, rather than blessing old tags.
					policy, policyErr := application.Repository().ScopeOptions()
					if policyErr != nil {
						err = policyErr
					} else {
						// Preserve the exact caller scope and ignore semantics: treating the
						// directory as an explicit positional bypasses .gitignore. The
						// planning pass already reported explicit-bypass notices; repeated
						// worker scans must not print duplicates or race on an output buffer.
						policy.Notices = nil
						paths, selectionErr := sourcedomain.ListWithPolicy(ctx, job.globs, root, policy)
						if selectionErr != nil {
							err = selectionErr
						} else {
							current, hashErr := directorymeta.FilesForDirectoryV1(root, job.directory, paths)
							if hashErr != nil {
								err = hashErr
							} else if !sameManifestV1(job.files, current) {
								err = fmt.Errorf("selected source changed during generation")
							}
						}
					}
					if err == nil {
						entry, err = retainCurrentUnselectedV1(root, job, entry)
						if err == nil {
							err = directorymeta.WriteDirectoryV1(root, job.directory, entry)
						}
					}
				}
				completed <- result{index, err}
			}
		}()
	}
	ready := make([]bool, len(plan))
	failures := make([]error, len(plan))
	for i, job := range plan {
		ready[i] = job.skip
	}
	next := 0
	var outputErrors []error
	flush := func() {
		for next < len(plan) && ready[next] {
			path := displayPath(root, plan[next].directory)
			if plan[next].skip {
				fmt.Fprintln(application.Stdout(), "skip", path)
			} else if failures[next] != nil {
				outputErrors = append(outputErrors, fmt.Errorf("generate %s: %w", path, failures[next]))
			} else {
				fmt.Fprintf(application.Stdout(), "write %s (%s)\n", filepath.ToSlash(directorymeta.RepositoryFile), path)
			}
			next++
		}
	}
	flush()
	for range pending {
		value := <-completed
		ready[value.index] = true
		failures[value.index] = value.err
		flush()
	}
	group.Wait()
	return errors.Join(outputErrors...)
}

func sameManifestV1(a, b []directorymeta.FileV1) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Checksum != b[i].Checksum {
			return false
		}
	}
	return true
}
