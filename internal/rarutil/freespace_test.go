package rarutil

import (
	"runtime"
	"testing"
)

func TestInsufficientSpace(t *testing.T) {
	tests := []struct {
		name    string
		avail   int64
		rarSize int64
		want    bool
	}{
		{"unknown avail never blocks", -1, 1 << 30, false},
		{"ample space", 10 << 30, 1 << 20, false},
		{"exactly equal is enough", 100, 100, false},
		{"below the archive size blocks", 50, 100, true},
		{"zero free blocks a nonempty archive", 0, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := insufficientSpace(tc.avail, tc.rarSize); got != tc.want {
				t.Errorf("insufficientSpace(%d, %d) = %v, want %v", tc.avail, tc.rarSize, got, tc.want)
			}
		})
	}
}

// TestAvailableBytes confirms the platform probe returns a sane value: a real,
// positive free-space figure on unix; the "unknown" sentinel elsewhere.
func TestAvailableBytes(t *testing.T) {
	got, err := availableBytes(t.TempDir())
	if err != nil {
		t.Fatalf("availableBytes: %v", err)
	}
	switch runtime.GOOS {
	case "windows", "plan9", "js", "wasip1":
		if got != -1 {
			t.Errorf("availableBytes = %d on %s, want -1 (unknown)", got, runtime.GOOS)
		}
	default: // unix
		if got <= 0 {
			t.Errorf("availableBytes = %d, want > 0 on a real temp filesystem", got)
		}
	}
}
