package rarutil

import "time"

// ProgressTracker accumulates cumulative byte/entry progress across an
// Extract or Test run so a caller (the CLI layer) can render a percentage
// and throughput without re-implementing cumulative-sum bookkeeping. It is
// pure arithmetic with no I/O of its own — the caller feeds it totals from a
// List() pre-pass and per-entry counts from Extract/Test's progress hooks.
type ProgressTracker struct {
	totalBytes   int64
	totalEntries int
	doneBytes    int64
	doneEntries  int
	start        time.Time
}

// NewProgressTracker starts a tracker for a run of totalBytes across
// totalEntries entries — both known ahead of time from a List() pre-pass
// (which must happen after password resolution on an encrypted archive).
func NewProgressTracker(totalBytes int64, totalEntries int) *ProgressTracker {
	return &ProgressTracker{totalBytes: totalBytes, totalEntries: totalEntries, start: time.Now()}
}

// OnEntry records one more completed entry, adding bytesWritten to the
// cumulative byte count.
func (p *ProgressTracker) OnEntry(bytesWritten int64) {
	p.doneBytes += bytesWritten
	p.doneEntries++
}

// Percent returns cumulative-bytes progress as 0-100, capped at 100. It
// returns 0 when totalBytes is unknown (<= 0) rather than dividing by zero.
func (p *ProgressTracker) Percent() float64 {
	if p.totalBytes <= 0 {
		return 0
	}
	pct := float64(p.doneBytes) / float64(p.totalBytes) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// ThroughputBPS returns bytes processed per second since the tracker
// started. It returns 0 when no measurable time has elapsed yet rather than
// dividing by zero.
func (p *ProgressTracker) ThroughputBPS() float64 {
	elapsed := time.Since(p.start).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(p.doneBytes) / elapsed
}
