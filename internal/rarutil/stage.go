package rarutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// commitMu serializes the commit step across every concurrent batch job so
// two jobs writing into the same destDir never interleave their
// check-then-rename pairs. The critical section is a fast walk+rename, not a
// slow decompression, so global serialization here is cheap — a per-destDir
// lock would be a premature optimization for this tool's typical batch sizes.
var commitMu sync.Mutex

// makeStagingDir creates a private staging directory inside destDir (creating
// destDir itself if needed) so the later commit's per-entry os.Rename is an
// O(1) same-filesystem move, not a byte copy. The staging dir is hidden
// (dot-prefixed) and uniquely named so concurrent batch jobs targeting the
// same destDir never collide.
func makeStagingDir(destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create destination %q: %w", destDir, err)
	}
	dir, err := os.MkdirTemp(destDir, ".macrarcli-staging-*")
	if err != nil {
		return "", fmt.Errorf("create staging dir under %q: %w", destDir, err)
	}
	return dir, nil
}

// commitEntry is one file successfully moved from staging into destDir during
// a commit walk, kept only so a later failure in the same walk can reverse it.
type commitEntry struct {
	stagingPath string
	destPath    string
}

// commitStaged moves every entry from stagingDir into destDir, applying opts'
// overwrite policy per file. On success it returns the list of archive-
// relative paths that were skipped under OverwriteSkip. On failure, every
// entry already committed in this walk is moved back into stagingDir and any
// directory this commit created is removed before the error is returned, so
// destDir ends up exactly as it was before the call. The caller is
// responsible for os.RemoveAll(stagingDir) afterward either way.
func commitStaged(stagingDir, destDir string, opts Options) (skipped []string, err error) {
	commitMu.Lock()
	defer commitMu.Unlock()

	committed := map[string]bool{}
	createdDirs := map[string]bool{} // destPath -> true if THIS commit created it
	var done []commitEntry

	walkErr := filepath.WalkDir(stagingDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == stagingDir {
			return nil
		}
		rel, relErr := filepath.Rel(stagingDir, p)
		if relErr != nil {
			return relErr
		}
		destPath := filepath.Join(destDir, rel)

		if d.IsDir() {
			fi, statErr := os.Lstat(destPath)
			switch {
			case errors.Is(statErr, fs.ErrNotExist):
				if mkErr := os.MkdirAll(destPath, 0o755); mkErr != nil {
					return fmt.Errorf("create %q: %w", destPath, mkErr)
				}
				createdDirs[destPath] = true
			case statErr != nil:
				return fmt.Errorf("stat %q: %w", destPath, statErr)
			case !fi.IsDir():
				// destPath exists but is NOT a real directory — most
				// dangerously, a symlink (e.g. destDir/shared -> /etc), which
				// os.MkdirAll/os.Rename would otherwise happily follow for
				// every entry nested under it. This is a hard safety
				// boundary, not an overwrite preference: no OverwritePolicy
				// value makes writing through a symlinked "directory" safe,
				// so it is rejected unconditionally, before any nested entry
				// is ever committed. filepath.WalkDir visits this staging
				// directory node before any of its children, so this check
				// runs — and can abort the whole walk — before any MkdirAll
				// or rename could occur beneath it.
				return fmt.Errorf("refusing to write into %q: exists and is not a directory (possible symlink)", destPath)
			}
			// destPath already exists and IS a real directory: nothing to
			// create, safe to let entries nested under it commit normally.
			return nil
		}

		finalDest, skip, ovErr := checkOverwrite(destPath, opts.OverwritePolicy, committed)
		if ovErr != nil {
			return ovErr
		}
		if skip {
			skipped = append(skipped, rel)
			return nil
		}
		if renameErr := renameOrCopy(p, finalDest); renameErr != nil {
			return fmt.Errorf("commit %q: %w", rel, renameErr)
		}
		done = append(done, commitEntry{stagingPath: p, destPath: finalDest})
		return nil
	})

	if walkErr != nil {
		for i := len(done) - 1; i >= 0; i-- {
			if revErr := renameOrCopy(done[i].destPath, done[i].stagingPath); revErr != nil {
				// Genuine reversal failure (not the benign "already gone"
				// case since we just created it): surface it, never swallow
				// silently — an entry may now be left behind in destDir.
				opts.vlog("rollback: failed to move %q back to staging: %v", done[i].destPath, revErr)
			}
		}
		removeCreatedDirsDeepestFirst(createdDirs, opts)
		return nil, walkErr
	}
	return skipped, nil
}

// removeCreatedDirsDeepestFirst removes directories this commit created, in
// deepest-first order, via plain os.Remove (never RemoveAll). A directory
// that has since gained unrelated content (e.g. a sibling entry's file still
// pending reversal, or — routinely on macOS — Finder/Spotlight metadata) is
// expected to be non-empty and simply fails to remove: that specific case is
// the safety property, not an error. Any OTHER removal error (permission
// denied, I/O error) is a genuine failure and is surfaced via opts.OnVerbose
// rather than swallowed uniformly.
func removeCreatedDirsDeepestFirst(createdDirs map[string]bool, opts Options) {
	dirs := make([]string, 0, len(createdDirs))
	for d := range createdDirs {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		if err := os.Remove(d); err != nil {
			// os.Remove doesn't expose a portable "directory not empty" error
			// value, so distinguish it directly: if the directory genuinely
			// still has content (a sibling entry pending reversal, or —
			// routinely on macOS — Finder/Spotlight metadata), that's the
			// expected, benign case. Anything else (permission denied, I/O
			// error) is a real failure and gets surfaced.
			if entries, readErr := os.ReadDir(d); readErr != nil || len(entries) == 0 {
				opts.vlog("rollback: failed to remove created directory %q: %v", d, err)
			}
		}
	}
}

// renameOrCopy moves src to dst, preferring an O(1) same-filesystem rename and
// falling back to copy+remove when that fails (e.g. dst crosses a filesystem
// boundary from src — rare, since makeStagingDir places staging next to
// destDir specifically to avoid this, but handled uniformly rather than left
// to fail outright).
func renameOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
