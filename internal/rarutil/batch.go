package rarutil

import (
	"errors"
	"sync"
)

// Job is a single source-RAR → destination-ZIP conversion.
type Job struct {
	Src string
	Dst string
}

// Result reports the outcome of one Job. Err is nil on success. Skipped is true
// when the job was intentionally not run (output already existed under
// --skip-existing); a skipped result has a nil Err and is not a failure.
type Result struct {
	Job
	Err     error
	Skipped bool
}

// RunBatch converts every job, continuing past failures so one bad archive
// never aborts the batch. Results are returned in the same order as jobs.
// At most maxParallel conversions run concurrently (values < 1 mean sequential);
// each job writes a distinct output file, so workers share no writer. opts
// (e.g. Password, Force) apply to every job. onStart, if non-nil, is called as
// each job begins — it may run from multiple goroutines, so it must be safe for
// concurrent use.
func RunBatch(jobs []Job, opts Options, maxParallel int, onStart func(Job)) []Result {
	if maxParallel < 1 {
		maxParallel = 1
	}
	results := make([]Result, len(jobs))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{} // block until a worker slot frees up
		go func(i int, j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			if onStart != nil {
				onStart(j)
			}
			// Distinct index per goroutine — no shared-slot write race.
			err := Convert(j.Src, j.Dst, opts)
			if errors.Is(err, ErrSkipped) {
				results[i] = Result{Job: j, Skipped: true}
			} else {
				results[i] = Result{Job: j, Err: err}
			}
		}(i, j)
	}

	wg.Wait()
	return results
}
