package rarutil

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
)

func TestCappedWriter_Overflow(t *testing.T) {
	for _, limit := range []int64{math.MaxInt64, 0} {
		var out bytes.Buffer
		total := int64(math.MaxInt64 - 1)
		w := &cappedWriter{w: &out, total: &total, limit: limit}
		if n, err := w.Write([]byte("xx")); n != 0 || !errors.Is(err, errLimitBytes) {
			t.Errorf("limit=%d: Write = %d, %v; want 0, errLimitBytes", limit, n, err)
		}
		if total != math.MaxInt64-1 || out.Len() != 0 {
			t.Errorf("rejected write changed total=%d or output=%q", total, out.String())
		}
		if n, err := w.Write([]byte("x")); n != 1 || err != nil || total != math.MaxInt64 {
			t.Errorf("exact boundary: %d, %v, total=%d", n, err, total)
		}
		if n, err := w.Write(nil); n != 0 || err != nil {
			t.Errorf("empty boundary write: %d, %v", n, err)
		}
	}
}

func TestCappedWriter_CountsActualWrites(t *testing.T) {
	total := int64(2)
	w := &cappedWriter{w: partialErrorWriter{}, total: &total, limit: 10}
	if n, err := w.Write([]byte("abcd")); n != 2 || !errors.Is(err, io.ErrClosedPipe) || total != 4 {
		t.Fatalf("Write = %d, %v, total=%d; want 2, ErrClosedPipe, 4", n, err, total)
	}
}

type partialErrorWriter struct{}

func (partialErrorWriter) Write(p []byte) (int, error) { return len(p) / 2, io.ErrClosedPipe }
