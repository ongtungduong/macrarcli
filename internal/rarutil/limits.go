package rarutil

import (
	"io"
	"math"
)

// cappedWriter forwards writes to w while accumulating *total, returning
// errLimitBytes (not a short write) the moment the running total would exceed
// limit. A zero limit disables the configured cap; totals must still fit int64.
type cappedWriter struct {
	w     io.Writer
	total *int64
	limit int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	limit := c.limit
	if limit <= 0 {
		limit = math.MaxInt64
	}
	if *c.total > limit || int64(len(p)) > limit-*c.total {
		return 0, errLimitBytes
	}
	n, err := c.w.Write(p)
	*c.total += int64(n)
	return n, err
}
