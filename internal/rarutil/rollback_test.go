package rarutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommitStaged_OverwriteRollbackRestoresOriginals(t *testing.T) {
	dest := t.TempDir()
	stage := testStagingDir(t, dest)
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dest, "a.txt")
	if err := os.WriteFile(old, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(old, modified, modified); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dest, "b.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dest, "z")); err != nil {
		t.Fatal(err)
	}
	writeStagingFile(t, stage, "a.txt", "replacement")
	writeStagingFile(t, stage, "b.txt", "replacement")
	writeStagingFile(t, stage, "new/file", "new")
	writeStagingFile(t, stage, "z/file", "trigger failure after replacements")
	_, err := commitStaged(stage, dest, Options{OverwritePolicy: OverwriteOverwrite})
	if err == nil {
		t.Fatal("expected late symlink-directory failure")
	}
	if got := readFile(t, old); got != "original" {
		t.Fatalf("lost original: %q", got)
	}
	info, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 || !info.ModTime().Equal(modified) {
		t.Fatalf("lost metadata: %v %v", info.Mode(), info.ModTime())
	}
	if got, err := os.Readlink(filepath.Join(dest, "b.txt")); err != nil || got != target {
		t.Fatalf("lost original symlink: %q %v", got, err)
	}
	if got := readFile(t, target); got != "untouched" {
		t.Fatal("wrote through symlink")
	}
	if _, err := os.Stat(filepath.Join(dest, "new")); !os.IsNotExist(err) {
		t.Fatalf("new directory left behind: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".macrarcli-backup-") {
			t.Fatalf("unnecessary backup retained: %s", e.Name())
		}
	}
}

func TestRollback_RestorationFailureKeepsBackup(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir("backup", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("backup/original", []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("file", []byte("concurrent data"), 0o600); err != nil {
		t.Fatal(err)
	}
	tx := commitTransaction{root: root, backupDir: "backup", done: []commitEntry{{dest: "file", backup: "backup/original"}}}
	if err := tx.rollback(); err == nil || !strings.Contains(err.Error(), "retained for recovery") {
		t.Fatalf("missing recovery error: %v", err)
	}
	if got := readFile(t, filepath.Join(dest, "file")); got != "concurrent data" {
		t.Fatal("clobbered concurrent file")
	}
	if got := readFile(t, filepath.Join(dest, "backup/original")); got != "original" {
		t.Fatal("deleted backup")
	}
}
