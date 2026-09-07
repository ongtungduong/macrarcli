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

// runParallel calls fn once per item and returns results in the same order as
// items (not completion order — each result is written to its own index, so
// there's no ordering race). At most maxParallel calls run concurrently
// (values < 1 mean sequential). Shared by every batch entry point below so
// the semaphore/WaitGroup fan-out logic exists in exactly one place.
func runParallel[T, R any](items []T, maxParallel int, fn func(T) R) []R {
	if maxParallel < 1 {
		maxParallel = 1
	}
	results := make([]R, len(items))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for i, item := range items {
		wg.Add(1)
		sem <- struct{}{} // block until a worker slot frees up
		go func(i int, item T) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = fn(item)
		}(i, item)
	}

	wg.Wait()
	return results
}

// RunBatch extracts every job, continuing past failures so one bad archive
// never aborts the batch. Results are returned in the same order as jobs. At
// most maxParallel extractions run concurrently (values < 1 mean sequential).
// opts (e.g. Password, OverwritePolicy) apply to every job. onStart, if
// non-nil, is called as each job begins — it may run from multiple
// goroutines, so it must be safe for concurrent use.
func RunBatch(jobs []Job, opts Options, maxParallel int, onStart func(Job)) []Result {
	return runParallel(jobs, maxParallel, func(j Job) Result {
		if onStart != nil {
			onStart(j)
		}
		skipped, err := Extract(j.Src, j.Dst, opts)
		return Result{Job: j, Err: err, SkippedEntries: skipped}
	})
}

// TestBatch validates every input archive (see Test), continuing past
// failures. Results are returned in the same order as srcs. At most
// maxParallel checks run concurrently (values < 1 mean sequential).
func TestBatch(srcs []string, opts Options, maxParallel int) []Result {
	return runParallel(srcs, maxParallel, func(src string) Result {
		err := Test(src, opts)
		return Result{Job: Job{Src: src}, Err: err}
	})
}

// ListBatch previews every input archive read-only (see List), continuing
// past failures. Results are returned in the same order as srcs. At most
// maxParallel listings run concurrently (values < 1 mean sequential).
func ListBatch(srcs []string, opts Options, maxParallel int) []ListResult {
	return runParallel(srcs, maxParallel, func(src string) ListResult {
		entries, err := List(src, opts)
		return ListResult{Src: src, Entries: entries, Err: err}
	})
}
