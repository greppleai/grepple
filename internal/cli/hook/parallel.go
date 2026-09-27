package hook

import (
	"context"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/greppleai/grepple/internal/gritql"
)

const (
	maxHookWorkers       = 4
	maxSharedScanFiles   = 100_000
	maxSharedScanBytes   = int64(1 << 30)
	maxSharedRuleResults = 10_000
	// Conservative serialized-finding ceiling; the shared scanner retains up
	// to 128 MiB per program including JSON, bindings, and matched source text.
	maxParallelResultBytes = int64(32 << 20)
)

type fileScanOutcome struct {
	index    int
	programs []gritql.ProgramScanResult
}

// scanProgramFiles evaluates distinct files concurrently while all queries
// on a single file still share one parse. Global scanner resource ceilings
// fall back to the original ordered batch path rather than becoming per-file
// ceilings; no incomplete parallel result is reported as a clean check.
func scanProgramFiles(ctx context.Context, filesystem fs.FS, programs []gritql.ProgramScan, candidates []gritql.ScanCandidate, options gritql.ScanOptions, workers int, all bool) ([][]gritql.ProgramScanResult, error) {
	serial := func() [][]gritql.ProgramScanResult {
		return [][]gritql.ProgramScanResult{gritql.ScanFilesPrograms(ctx, filesystem, programs, candidates, options).Programs()}
	}
	if workers <= 1 || len(candidates) <= 1 || !parallelSourceBudget(filesystem, candidates) {
		return serial(), nil
	}
	if workers > len(candidates) {
		workers = len(candidates)
	}
	deadline := 30 * time.Second
	if all {
		deadline = 300 * time.Second
	}
	groupCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	jobs := make(chan int)
	outcomes := make(chan fileScanOutcome, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for index := range jobs {
				fileOptions := options
				fileOptions.Workers = 1
				result := gritql.ScanFilesPrograms(groupCtx, filesystem, programs, candidates[index:index+1], fileOptions)
				outcomes <- fileScanOutcome{index: index, programs: result.Programs()}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range candidates {
			select {
			case jobs <- index:
			case <-groupCtx.Done():
				return
			}
		}
	}()
	go func() {
		group.Wait()
		close(outcomes)
	}()
	rows := make([][]gritql.ProgramScanResult, len(candidates))
	for outcome := range outcomes {
		rows[outcome.index] = outcome.programs
	}
	if err := groupCtx.Err(); err != nil {
		return nil, fmt.Errorf("hook scan incomplete: %w", err)
	}
	for _, row := range rows {
		if len(row) != len(programs) {
			return nil, fmt.Errorf("hook scan incomplete: not all files were evaluated")
		}
	}
	limit := maxSharedRuleResults
	if options.EvaluateOptions.MaxFindings > limit && options.EvaluateOptions.MaxFindings <= 100_000 {
		limit = options.EvaluateOptions.MaxFindings
	}
	for index := range programs {
		count := 0
		var retainedUpperBound int64
		for _, row := range rows {
			for _, finding := range row[index].Result.Findings() {
				count++
				encoded, err := finding.MarshalJSON()
				if err != nil {
					return serial(), nil
				}
				retainedUpperBound += int64(5*len(encoded) + 2*len(finding.Text()) + 512)
			}
			for _, diagnostic := range row[index].Result.Diagnostics() {
				encoded, err := diagnostic.MarshalJSON()
				if err != nil {
					return serial(), nil
				}
				retainedUpperBound += int64(5*len(encoded) + 512)
			}
			if count > limit || retainedUpperBound > maxParallelResultBytes {
				return serial(), nil
			}
		}
	}
	return rows, nil
}

func parallelSourceBudget(filesystem fs.FS, candidates []gritql.ScanCandidate) bool {
	if len(candidates) > maxSharedScanFiles {
		return false
	}
	var total int64
	for _, candidate := range candidates {
		if candidate.Content != nil {
			total += int64(len(candidate.Content))
		} else {
			info, err := fs.Stat(filesystem, candidate.ReadPath)
			if err != nil || !info.Mode().IsRegular() {
				return false
			}
			total += info.Size()
		}
		if total > maxSharedScanBytes {
			return false
		}
	}
	return true
}
