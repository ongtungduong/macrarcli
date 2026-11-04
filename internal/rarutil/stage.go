package rarutil

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Commit serialization protects jobs in this process. Root-relative operations
// also prevent a concurrent symlink swap from escaping the destination tree.
var commitMu sync.Mutex

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

func makeRootTempDir(root *os.Root, prefix string) (string, error) {
	for {
		name := prefix + rand.Text()
		if err := root.Mkdir(name, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return "", err
		}
		return name, nil
	}
}

// commitStaged requires staging directly under destDir, as makeStagingDir
// guarantees. Originals replaced by --overwrite stay in a private backup
// directory until success. Failed restoration preserves that directory and
// returns its location, rather than deleting the only remaining original.
func commitStaged(stagingDir, destDir string, opts Options) ([]string, error) {
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	rel, err := filepath.Rel(destDir, stagingDir)
	if err != nil || rel == "." || filepath.Base(rel) != rel || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("staging must be directly inside destination")
	}
	return commitRoot(root, rel, opts)
}

func commitRoot(root *os.Root, staging string, opts Options) ([]string, error) {
	stage, err := root.OpenRoot(staging)
	if err != nil {
		return nil, err
	}
	defer stage.Close()
	return commitPinned(root, staging, stage, opts)
}

func commitPinned(root *os.Root, staging string, stage *os.Root, opts Options) ([]string, error) {
	commitMu.Lock()
	defer commitMu.Unlock()
	expected, err := stage.Stat(".")
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(staging)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(current, expected) {
		return nil, fmt.Errorf("staging path changed concurrently")
	}
	tx := &commitTransaction{root: root}
	defer tx.close()
	resolver := newOverwriteResolver(root.Lstat)
	var skipped []string
	walkErr := fs.WalkDir(stage.FS(), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == "." {
			return nil
		}
		rel := filepath.FromSlash(p)
		first := strings.SplitN(p, "/", 2)[0]
		if first == staging || first == tx.backupDir {
			return fmt.Errorf("entry conflicts with private transaction directory: %q", p)
		}
		if d.IsDir() {
			fi, statErr := root.Lstat(rel)
			switch {
			case errors.Is(statErr, fs.ErrNotExist):
				if err := root.Mkdir(rel, 0o755); err != nil {
					return err
				}
				created, err := root.Lstat(rel)
				if err != nil {
					return err
				}
				if !created.IsDir() {
					return fmt.Errorf("created directory %q changed concurrently", rel)
				}
				tx.createdDirs = append(tx.createdDirs, commitDir{rel, created})
			case statErr != nil:
				return statErr
			case !fi.IsDir():
				return fmt.Errorf("refusing to write into %q: exists and is not a directory (possible symlink)", rel)
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("staged entry is not a regular file: %q", p)
		}
		final, skip, err := resolver.resolve(rel, opts.OverwritePolicy)
		if err != nil {
			return err
		}
		if skip {
			skipped = append(skipped, p)
			return nil
		}
		entry := commitEntry{dest: final}
		if opts.OverwritePolicy == OverwriteOverwrite {
			existing, statErr := root.Lstat(final)
			switch {
			case statErr == nil:
				if !existing.Mode().IsRegular() && existing.Mode()&fs.ModeSymlink == 0 {
					return fmt.Errorf("refusing to replace non-file %q", final)
				}
				entry.backup, err = tx.backupFile(final, existing)
				if err != nil {
					return err
				}
			case !errors.Is(statErr, fs.ErrNotExist):
				return statErr
			}
		}
		// Record the backup before publication; even a failed write must restore it.
		tx.done = append(tx.done, entry)
		published, err := publishFrom(stage, rel, root, final)
		tx.done[len(tx.done)-1].published = published
		if err != nil {
			return fmt.Errorf("commit %q: %w", p, err)
		}
		return nil
	})
	if walkErr != nil {
		return nil, errors.Join(walkErr, tx.rollback())
	}
	if tx.backupDir != "" {
		if err := cleanupStaging(root, tx.backupDir, tx.backupRoot); err != nil {
			return skipped, fmt.Errorf("output committed; could not remove backups at %q: %w", filepath.Join(root.Name(), tx.backupDir), err)
		}
	}
	return skipped, nil
}
