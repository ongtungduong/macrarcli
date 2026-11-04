package rarutil

import (
	"golang.org/x/sys/windows"
	"os"
	"time"
)

func setFileTimes(f *os.File, t time.Time) error {
	ft := windows.NsecToFiletime(t.UnixNano())
	return windows.SetFileTime(windows.Handle(f.Fd()), nil, &ft, &ft)
}
