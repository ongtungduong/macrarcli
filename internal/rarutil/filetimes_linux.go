package rarutil

import (
	"golang.org/x/sys/unix"
	"os"
	"time"
)

func setFileTimes(f *os.File, t time.Time) error {
	ts := unix.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
	return unix.UtimesNanoAt(int(f.Fd()), "", []unix.Timespec{ts, ts}, unix.AT_EMPTY_PATH)
}
