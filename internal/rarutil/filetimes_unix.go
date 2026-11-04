//go:build unix && !linux && !darwin

package rarutil

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// Apply metadata to the opened file, never to a path that can become a symlink.
func setFileTimes(f *os.File, t time.Time) error {
	// Avoid UnixNano overflow for valid archive dates outside 1678–2262.
	tv := unix.NsecToTimeval(int64(t.Nanosecond()/1000) * 1000)
	tv.Sec = t.Unix()
	return unix.Futimes(int(f.Fd()), []unix.Timeval{tv, tv})
}
