package rarutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCopyExclusive_MetadataAndFailureRollback(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("source", []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	modified := time.Date(2020, 1, 2, 3, 4, 5, 123456700, time.UTC)
	if err := root.Chtimes("source", modified, modified); err != nil {
		t.Fatal(err)
	}
	info, err := root.Stat("source")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := copyExclusive(strings.NewReader("data"), info, root, "copy")
	if err != nil || identity == nil {
		t.Fatalf("copy: %v %v", identity, err)
	}
	copied, err := root.Stat("copy")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Mode().Perm() != 0o640 || !copied.ModTime().Equal(modified) {
		t.Fatal("metadata lost")
	}
	if _, err := copyExclusive(strings.NewReader("bad"), info, root, "copy"); !os.IsExist(err) {
		t.Fatalf("exclusive copy: %v", err)
	}
	if got, err := root.ReadFile("copy"); err != nil || string(got) != "data" {
		t.Fatalf("overwritten: %q %v", got, err)
	}
	boom := errors.New("copy failed")
	identity, err = copyExclusive(&errReader{err: boom}, info, root, "partial")
	if !errors.Is(err, boom) || identity == nil {
		t.Fatalf("partial identity not returned: %v %v", identity, err)
	}
	tx := commitTransaction{root: root, done: []commitEntry{{dest: "partial", published: identity}}}
	if err := tx.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("partial"); !os.IsNotExist(err) {
		t.Fatalf("partial survived: %v", err)
	}
}

func TestRecoveryRootReplacementIsNotDeleted(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	tx := commitTransaction{root: root}
	defer tx.close()
	if err := tx.recoveryDir(); err != nil {
		t.Fatal(err)
	}
	if err := tx.backupRoot.WriteFile("original", []byte("keep original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename(tx.backupDir, "moved"); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir(tx.backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(filepath.Join(tx.backupDir, "unrelated"), []byte("keep unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cleanupStaging(root, tx.backupDir, tx.backupRoot); err == nil {
		t.Fatal("accepted replacement")
	}
	if got := readFile(t, filepath.Join(dest, "moved/original")); got != "keep original" {
		t.Fatal("lost original")
	}
	if got := readFile(t, filepath.Join(dest, tx.backupDir, "unrelated")); got != "keep unrelated" {
		t.Fatal("lost replacement")
	}
}

func TestRemoveEmptyDirNeverDeletesFile(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.WriteFile("file", []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeEmptyDir(root, "file"); err == nil {
		t.Fatal("accepted file as directory")
	}
	if got, err := root.ReadFile("file"); err != nil || string(got) != "unrelated" {
		t.Fatalf("deleted file: %q %v", got, err)
	}
}
