package rarutil

import (
	"math"
	"testing"
)

func TestProgressTracker_Percent(t *testing.T) {
	tests := []struct {
		name       string
		totalBytes int64
		doneBytes  int64
		want       float64
	}{
		{"zero_percent", 100, 0, 0},
		{"fifty_percent", 100, 50, 50},
		{"hundred_percent", 100, 100, 100},
		{"over_hundred_capped", 100, 150, 100},
		{"zero_total_no_div_by_zero", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProgressTracker(tt.totalBytes, 1)
			p.doneBytes = tt.doneBytes
			if got := p.Percent(); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("Percent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProgressTracker_OnEntryAccumulates(t *testing.T) {
	p := NewProgressTracker(300, 3)
	p.OnEntry(100)
	p.OnEntry(100)
	p.OnEntry(100)
	if p.doneEntries != 3 {
		t.Errorf("doneEntries = %d, want 3", p.doneEntries)
	}
	if got := p.Percent(); got != 100 {
		t.Errorf("Percent() after 3 entries = %v, want 100", got)
	}
}

// TestProgressTracker_ThroughputNoDivByZero proves ThroughputBPS never
// panics or returns NaN/Inf regardless of how little wall-clock time has
// elapsed since the tracker started (real elapsed time is never exactly
// zero, but the guard exists for a zero/negative-elapsed edge case a
// low-resolution clock could produce).
func TestProgressTracker_ThroughputNoDivByZero(t *testing.T) {
	p := NewProgressTracker(100, 1)
	p.OnEntry(50)
	got := p.ThroughputBPS()
	if got < 0 || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("ThroughputBPS() = %v, want a finite non-negative value", got)
	}
}
