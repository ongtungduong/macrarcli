//go:build !unix

package rarutil

import (
	"errors"
	"os"
)

// Use exclusive copying when descriptor-relative linking is unavailable.
func linkBetweenRoots(src *os.Root, name string, dst *os.Root, finalName string) error {
	return errors.New("descriptor-relative hard linking unavailable")
}
