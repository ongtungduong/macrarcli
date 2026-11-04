//go:build !unix && !windows

package rarutil

import (
	"errors"
	"os"
)

func removeEmptyDir(root *os.Root, name string) error {
	return errors.New("safe directory removal unsupported on this platform")
}
