//go:build unix

package rarutil

import "syscall"

// availableBytes returns the free space (bytes available to an unprivileged
// process) on the filesystem backing dir, for the fallback path's lightweight
// pre-extraction check.
func availableBytes(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
