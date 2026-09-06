package rarutil

import "sync"

// Job is a single source-RAR -> destination-directory extraction. Dst names a
// directory (Extract's target), not a file — multiple jobs MAY target the
// same Dst; commitStaged's package-level lock serializes their commit steps
// so concurrent jobs never race on the same destination path.
type Job struct {
	Src string
	Dst string
}

// Result reports the outcome of one Job. Err is nil on success.
// SkippedEntries lists archive-relative paths that were individually skipped
// under the OverwriteSkip policy — a job with only skips and no Err is still
// a success, just a partial one.
type Result struct {
	Job
	Err            error
	SkippedEntries []string
}

// RunBatch extracts every job, continuing past failures so one bad archive
// never aborts the batch. Results are returned in the same order as jobs. At
// most maxParallel extractions run concurrently (values < 1 mean sequential).
// opts (e.g. Password, OverwritePolicy) apply to every job. onStart, if
// non-nil, is called as each job begins — it may run from multiple
// goroutines, so it must be safe for concurrent use.
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
			skipped, err := Extract(j.Src, j.Dst, opts)
			results[i] = Result{Job: j, Err: err, SkippedEntries: skipped}
		}(i, j)
	}

	wg.Wait()
	return results
}
