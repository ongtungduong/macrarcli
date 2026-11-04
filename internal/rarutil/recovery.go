package rarutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type commitDir struct {
	name string
	info fs.FileInfo
}

// SameFile compares device/inode only. Linux can reuse a freed inode for a
// different object type, so a file-to-directory substitution must also fail.
func sameObject(actual, expected fs.FileInfo) bool {
	return actual != nil && expected != nil && actual.Mode().Type() == expected.Mode().Type() && os.SameFile(actual, expected)
}

func (tx *commitTransaction) recoveryDir() error {
	if tx.backupRoot != nil {
		return nil
	}
	if tx.backupDir == "" {
		name, err := makeRootTempDir(tx.root, ".macrarcli-backup-")
		if err != nil {
			return err
		}
		tx.backupDir = name
	}
	var err error
	tx.backupRoot, err = tx.root.OpenRoot(tx.backupDir)
	return err
}

// Inspect after moving privately: a concurrent directory substitution must
// never be recursively deleted because an earlier Lstat saw a regular file.
func (tx *commitTransaction) backupFile(name string, expected fs.FileInfo) (string, error) {
	if err := tx.recoveryDir(); err != nil {
		return "", err
	}
	backup := filepath.Join(tx.backupDir, fmt.Sprintf("original-%d", len(tx.done)))
	if err := tx.root.Rename(name, backup); err != nil {
		return "", err
	}
	got, err := tx.backupRoot.Lstat(filepath.Base(backup))
	if err != nil || !sameObject(got, expected) {
		tx.preserveBackup = true
		return "", fmt.Errorf("backup %q changed concurrently; retained at %q: %w", name, filepath.Join(tx.root.Name(), backup), errors.Join(err, fs.ErrInvalid))
	}
	return backup, nil
}

func (tx *commitTransaction) removePublished(name string, expected fs.FileInfo, index int) error {
	if err := tx.recoveryDir(); err != nil {
		return err
	}
	quarantine := filepath.Join(tx.backupDir, fmt.Sprintf("rollback-%d", index))
	if err := tx.root.Rename(name, quarantine); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	got, err := tx.backupRoot.Lstat(filepath.Base(quarantine))
	if err != nil {
		return err
	}
	if !sameObject(got, expected) {
		return fmt.Errorf("rollback: %q changed concurrently; retained at %q", name, filepath.Join(tx.root.Name(), quarantine))
	}
	// Nonrecursive even for directories: unrelated contents must survive.
	return tx.backupRoot.Remove(filepath.Base(quarantine))
}

// Compare the held root's identity again after moving it privately. Never
// recursively delete a replacement at the original staging pathname.
func cleanupStaging(root *os.Root, name string, stage *os.Root) error {
	expected, err := stage.Stat(".")
	if err != nil {
		return err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !sameObject(current, expected) {
		return fmt.Errorf("staging path %q changed concurrently; preserved for recovery", name)
	}
	private, err := makeRootTempDir(root, ".macrarcli-cleanup-")
	if err != nil {
		return err
	}
	privateRoot, err := root.OpenRoot(private)
	if err != nil {
		return err
	}
	defer privateRoot.Close()
	moved := filepath.Join(private, "staging")
	if err := root.Rename(name, moved); err != nil {
		removeEmptyDir(root, private)
		return err
	}
	current, err = privateRoot.Lstat("staging")
	if err != nil || !sameObject(current, expected) {
		return fmt.Errorf("staging changed concurrently; retained at %q: %w", filepath.Join(root.Name(), moved), errors.Join(err, fs.ErrInvalid))
	}
	if err := privateRoot.RemoveAll("staging"); err != nil {
		return err
	}
	return removeEmptyDir(root, private)
}
