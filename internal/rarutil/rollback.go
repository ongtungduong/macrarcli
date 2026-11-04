package rarutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type commitEntry struct {
	dest      string
	backup    string
	published fs.FileInfo
}

type commitTransaction struct {
	root           *os.Root
	backupDir      string
	backupRoot     *os.Root
	done           []commitEntry
	createdDirs    []commitDir
	preserveBackup bool
}

func (tx *commitTransaction) rollback() error {
	defer tx.close()
	var failures []error
	for i := len(tx.done) - 1; i >= 0; i-- {
		e := tx.done[i]
		if e.published != nil {
			if err := tx.removePublished(e.dest, e.published, i); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		if e.backup != "" {
			// Link never replaces a concurrently created path and does not follow a
			// symlink backup. A failed restore leaves the original in the backup dir.
			if err := tx.recoveryDir(); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := restoreBackupFrom(tx.backupRoot, filepath.Base(e.backup), tx.root, e.dest); err != nil {
				failures = append(failures, fmt.Errorf("restore %q: %w", e.dest, err))
			} else if err := tx.backupRoot.Remove(filepath.Base(e.backup)); err != nil {
				failures = append(failures, err)
			}
		}
	}
	for i := len(tx.createdDirs) - 1; i >= 0; i-- {
		dir := tx.createdDirs[i]
		if err := tx.removePublished(dir.name, dir.info, len(tx.done)+i); err != nil {
			failures = append(failures, err)
		}
	}
	if tx.preserveBackup {
		failures = append(failures, errors.New("unexpected backup object preserved"))
	}

	if len(failures) != 0 {
		if tx.backupDir != "" {
			failures = append(failures, fmt.Errorf("data retained for recovery at %q", filepath.Join(tx.root.Name(), tx.backupDir)))
		}
		return errors.Join(failures...)
	}
	if tx.backupDir != "" {
		return cleanupStaging(tx.root, tx.backupDir, tx.backupRoot)
	}
	return nil
}

// restoreBackup preserves symlinks themselves and supports filesystems without
// hard links using the same exclusive publication fallback as extraction.
func restoreBackupFrom(source *os.Root, backup string, destination *os.Root, dest string) error {
	info, err := source.Lstat(backup)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := source.Readlink(backup)
		if err != nil {
			return err
		}
		return destination.Symlink(target, dest)
	}
	_, err = publishFrom(source, backup, destination, dest)
	return err
}

func (tx *commitTransaction) close() {
	if tx.backupRoot != nil {
		tx.backupRoot.Close()
	}
}
