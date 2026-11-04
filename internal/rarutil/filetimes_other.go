//go:build !unix && !windows

package rarutil

import (
	"errors"
	"os"
	"time"
)

func setFileTimes(f *os.File, t time.Time) error {
	return errors.New("safe file timestamps unsupported on this platform")
}
