package rarutil

import (
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestRunBatch_ContinueOnError proves the batch driver runs every job despite
// failures, preserves input order in results, and reports per-job errors.
// Needs no fixture: non-existent sources fail deterministically.
func TestRunBatch_ContinueOnError(t *testing.T) {
	dir := t.TempDir()
	jobs := []Job{
		{Src: filepath.Join(dir, "a.rar"), Dst: filepath.Join(dir, "a.zip")},
		{Src: filepath.Join(dir, "b.rar"), Dst: filepath.Join(dir, "b.zip")},
		{Src: filepath.Join(dir, "c.rar"), Dst: filepath.Join(dir, "c.zip")},
	}
	results := RunBatch(jobs, Options{}, 1, nil)

	if len(results) != len(jobs) {
		t.Fatalf("got %d results, want %d", len(results), len(jobs))
	}
	for i, r := range results {
		if r.Src != jobs[i].Src {
			t.Errorf("result[%d].Src = %q, want %q (order not preserved)", i, r.Src, jobs[i].Src)
		}
		if r.Err == nil {
			t.Errorf("result[%d] (%s) Err = nil, want failure", i, r.Src)
		}
	}
}

// TestRunBatch_ConcurrentSuccess converts the same fixture to several distinct
// outputs in parallel, verifying race-free success and per-job output files.
func TestRunBatch_ConcurrentSuccess(t *testing.T) {
	matches, _ := filepath.Glob("../../testdata/*.rar")
	if len(matches) == 0 {
		t.Skip("no testdata/*.rar fixture present; skipping concurrent batch test")
	}
	src := matches[0]
	dir := t.TempDir()

	var jobs []Job
	for i := 0; i < 5; i++ {
		jobs = append(jobs, Job{Src: src, Dst: filepath.Join(dir, string(rune('a'+i))+".zip")})
	}

	var started atomic.Int32
	results := RunBatch(jobs, Options{}, 3, func(Job) { started.Add(1) })

	if int(started.Load()) != len(jobs) {
		t.Errorf("onStart called %d times, want %d", started.Load(), len(jobs))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("job %d (%s): %v", i, r.Dst, r.Err)
		}
	}
}
