package search

import (
	"os"
	"runtime"
	"strconv"
	"sync"
)

const maxDefaultWorkers = 8

var (
	workerOnce   sync.Once
	workerTokens chan struct{}
)

// WorkerCount reports the process-wide search concurrency limit. Set
// GREPPLE_WORKERS to override the default of min(GOMAXPROCS, 8).
func WorkerCount() int {
	workerOnce.Do(func() {
		count := runtime.GOMAXPROCS(0)
		if count > maxDefaultWorkers {
			count = maxDefaultWorkers
		}
		if configured, err := strconv.Atoi(os.Getenv("GREPPLE_WORKERS")); err == nil && configured > 0 {
			count = configured
		}
		if count < 1 {
			count = 1
		}
		workerTokens = make(chan struct{}, count)
	})
	return cap(workerTokens)
}

func runParallel(count int, task func(int)) {
	if count <= 0 {
		return
	}
	workers := min(count, WorkerCount())
	jobs := make(chan int)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			workerTokens <- struct{}{}
			defer func() {
				<-workerTokens
				wait.Done()
			}()
			for index := range jobs {
				task(index)
			}
		}()
	}
	for index := range count {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
}
