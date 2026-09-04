//go:build !unix

package rarutil

// availableBytes reports "unknown" (-1) on platforms without a portable statfs.
// The caller treats unknown as "do not block" and relies on the extractor's own
// out-of-space error as the hard bound.
func availableBytes(dir string) (int64, error) {
	return -1, nil
}
