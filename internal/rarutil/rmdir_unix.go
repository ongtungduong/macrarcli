//go:build unix

package rarutil

import (
	"golang.org/x/sys/unix"
	"os"
)

// name is one component. Never remove a file substituted for the directory.
func removeEmptyDir(root *os.Root, name string) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
}
