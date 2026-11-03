package directorymeta

import "sync"

// inspectInventoryDirectoriesV1 overlaps checksum/existence reads without
// weakening current-content validation. Workers only read the shared snapshot
// and write their own result slot; the caller merges in directory order.
func inspectInventoryDirectoriesV1(root string, directories []string, paths map[string][]string, document DocumentV1, loadErr error) []InspectionV1 {
	inspections := make([]InspectionV1, len(directories))
	inspect := func(index int) {
		directory := directories[index]
		inspections[index] = inspectDirectorySnapshotV1(root, directory, paths[directory], document, loadErr)
	}
	// Missing/invalid snapshots need no source I/O. Keep unknown-directory
	// orientation cheap, and avoid workers for single-directory selections.
	if loadErr != nil || len(directories) < 2 {
		for index := range directories {
			inspect(index)
		}
		return inspections
	}
	// Bound simultaneous file reads even in large multi-checkout trees.
	workers := min(8, len(directories))
	jobs := make(chan int)
	var done sync.WaitGroup
	for range workers {
		done.Add(1)
		go func() {
			defer done.Done()
			for index := range jobs {
				inspect(index)
			}
		}()
	}
	for index := range directories {
		jobs <- index
	}
	close(jobs)
	done.Wait()
	return inspections
}
