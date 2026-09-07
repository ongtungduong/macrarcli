package rarutil

import (
	"fmt"
	"io/fs"
	"os"
)

// OverwritePolicy controls what happens when a commit-time destination path
// already exists on disk (or was already claimed earlier in the same commit
// walk). The zero value (OverwriteFail) is the default: abort rather than
// silently clobber anything.
type OverwritePolicy int

const (
	// OverwriteFail aborts the commit on the first collision (default).
	OverwriteFail OverwritePolicy = iota
	// OverwriteOverwrite clobbers the existing path.
	OverwriteOverwrite
	// OverwriteSkip skips just the colliding entry, keeping the rest of the run.
	OverwriteSkip
	// OverwriteRename writes the entry under a " (k)" suffixed variant instead.
	OverwriteRename
)

// checkOverwrite decides what path (if any) an entry should commit to, given
// the destination path it would naturally land at, the configured policy,
// and the set of paths already claimed earlier in THIS commit walk
// (committed — shared across every entry in one commitStaged call, so a
// --rename output can never silently collide with a later entry in the same
// run). It always uses os.Lstat — never os.Stat — so a symlink planted at the
// destination is treated as "occupied," never followed and never written
// through.
func checkOverwrite(destPath string, policy OverwritePolicy, committed map[string]bool) (finalPath string, skip bool, err error) {
	if !committed[destPath] {
		if _, statErr := os.Lstat(destPath); os.IsNotExist(statErr) {
			committed[destPath] = true
			return destPath, false, nil
		}
	}

	switch policy {
	case OverwriteOverwrite:
		if fi, statErr := os.Lstat(destPath); statErr == nil && fi.Mode()&fs.ModeSymlink != 0 {
			// Never write through a symlink: remove it first, then treat the
			// path as free. A regular file at destPath is left for os.Rename
			// (or the copy+remove fallback) to replace directly.
			if rmErr := os.Remove(destPath); rmErr != nil {
				return "", false, fmt.Errorf("remove symlink at %q before overwrite: %w", destPath, rmErr)
			}
		}
		committed[destPath] = true
		return destPath, false, nil
	case OverwriteSkip:
		return "", true, nil
	case OverwriteRename:
		for k := 1; ; k++ {
			cand := dedupVariant(destPath, k)
			if committed[cand] {
				continue
			}
			if _, statErr := os.Lstat(cand); os.IsNotExist(statErr) {
				committed[cand] = true
				return cand, false, nil
			}
		}
	default: // OverwriteFail
		return "", false, fmt.Errorf("output exists (use --overwrite to replace, --skip to skip, or --rename to keep both): %s", destPath)
	}
}
