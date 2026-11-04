package rarutil

import (
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"time"
	"unsafe"
)

// fsetattrlist supports descriptor-based nanosecond timestamps on macOS.
func setFileTimes(f *os.File, t time.Time) error {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_MODTIME | unix.ATTR_CMN_ACCTIME}
	ts := unix.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
	times := [2]unix.Timespec{ts, ts}
	_, _, errno := unix.Syscall6(unix.SYS_FSETATTRLIST, f.Fd(), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&times)), unsafe.Sizeof(times), 0, 0)
	runtime.KeepAlive(f)
	if errno != 0 {
		return errno
	}
	return nil
}
