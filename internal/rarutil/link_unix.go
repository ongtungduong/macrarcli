//go:build unix

package rarutil

import (
	"golang.org/x/sys/unix"
	"os"
)

// Names are single components beneath pinned parents. Never resolve the
// original staging pathname again; Linkat does not follow the source link.
func linkBetweenRoots(src *os.Root, name string, dst *os.Root, finalName string) error {
	from, err := src.Open(".")
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := dst.Open(".")
	if err != nil {
		return err
	}
	defer to.Close()
	return unix.Linkat(int(from.Fd()), name, int(to.Fd()), finalName, 0)
}
